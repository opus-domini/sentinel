package mcpserver

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	opsplane "github.com/opus-domini/sentinel/internal/services"
)

const (
	sensorCategoryTemperature = "temperature"
	sensorCategoryFan         = "fan"
	sensorCategoryPower       = "power"

	// defaultSensorLimit returns an ordinary host whole. A 24-core laptop
	// reports 34 temperatures and a 1U dual-socket 32-core server about 94.
	// Only coretemp scales with the machine, at one reading per physical core
	// per socket, so a host above this is a large Intel part whose extra rows
	// are near-identical cores that one sources row already describes.
	defaultSensorLimit = 100
	// maxSensorLimit keeps the largest response a caller can ask for one that
	// still arrives intact. 250 sensors is roughly 36 KB, and the MCP SDK sends
	// the payload a second time as text, which lands just inside the 25,000
	// token budget clients allow one tool result.
	maxSensorLimit = 250
)

// metricsSource reads one host sample. *services.Manager satisfies it.
type metricsSource interface {
	MetricsSnapshot(context.Context) opsplane.MetricsSnapshot
}

type listSensorsInput struct {
	Category string   `json:"category,omitempty" jsonschema:"optional sensor category: temperature, fan, or power; every category is returned when omitted"`
	Status   []string `json:"status,omitempty" jsonschema:"optional statuses to keep: ok, warning, critical, or unknown; every status is kept when omitted"`
	Match    string   `json:"match,omitempty" jsonschema:"optional case-insensitive substring matched against sensor id, label, or source"`
	Limit    int      `json:"limit,omitempty" jsonschema:"maximum individual sensors returned per category after worst-first ranking; the sources rollup is never truncated; defaults to 100 and caps at 250"`
}

type temperatureSensorOutput struct {
	ID              string   `json:"id"`
	Label           string   `json:"label"`
	Source          string   `json:"source"`
	Status          string   `json:"status"`
	Celsius         float64  `json:"celsius"`
	MaxCelsius      *float64 `json:"maxCelsius,omitempty"`
	CriticalCelsius *float64 `json:"criticalCelsius,omitempty"`
	Alarm           *bool    `json:"alarm,omitempty"`
}

type fanSensorOutput struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Source string `json:"source"`
	Status string `json:"status"`
	RPM    int64  `json:"rpm"`
	MinRPM *int64 `json:"minRpm,omitempty"`
	MaxRPM *int64 `json:"maxRpm,omitempty"`
	Alarm  *bool  `json:"alarm,omitempty"`
}

type powerSensorOutput struct {
	ID            string   `json:"id"`
	Label         string   `json:"label"`
	Source        string   `json:"source"`
	Status        string   `json:"status"`
	Watts         float64  `json:"watts"`
	MaxWatts      *float64 `json:"maxWatts,omitempty"`
	CriticalWatts *float64 `json:"criticalWatts,omitempty"`
	CapWatts      *float64 `json:"capWatts,omitempty"`
	Alarm         *bool    `json:"alarm,omitempty"`
}

// sensorSummaryOutput describes every sensor the host reported, before any
// filter, so a narrow query still reveals pressure it excluded.
type sensorSummaryOutput struct {
	Total    int                      `json:"total"`
	Warning  int                      `json:"warning"`
	Critical int                      `json:"critical"`
	Unknown  int                      `json:"unknown"`
	Hottest  *temperatureSensorOutput `json:"hottest,omitempty"`
}

// sensorSourceOutput rolls every sensor one chip reported in one category into
// a single row. Chip names repeat -- a 24-drive server reports 24 chips all
// named nvme -- so one row describes a whole population and WorstID names the
// member worth reading. A rollup costs one row per chip rather than one per
// sensor, which is the same handful of rows on a laptop, a bare VM and a
// 288-core server, so it is never truncated.
type sensorSourceOutput struct {
	Category string  `json:"category"`
	Source   string  `json:"source"`
	Count    int     `json:"count"`
	Status   string  `json:"status"`
	Warning  int     `json:"warning,omitempty"`
	Critical int     `json:"critical,omitempty"`
	Unknown  int     `json:"unknown,omitempty"`
	Lowest   float64 `json:"lowest"`
	Average  float64 `json:"average"`
	Highest  float64 `json:"highest"`
	WorstID  string  `json:"worstId"`
}

type listSensorsOutput struct {
	Platform     string                    `json:"platform"`
	CollectedAt  string                    `json:"collectedAt"`
	Summary      sensorSummaryOutput       `json:"summary"`
	Matched      int                       `json:"matched"`
	Returned     int                       `json:"returned"`
	Truncated    bool                      `json:"truncated"`
	Sources      []sensorSourceOutput      `json:"sources"`
	Temperatures []temperatureSensorOutput `json:"temperatures"`
	Fans         []fanSensorOutput         `json:"fans"`
	Power        []powerSensorOutput       `json:"power"`
}

type sensorFilter struct {
	category string
	statuses []string
	match    string
	limit    int
}

// sensorOrder is the ranking key shared by all three categories.
type sensorOrder struct {
	status string
	value  float64
	id     string
}

func (t *tools) listSensors(ctx context.Context, _ *mcp.CallToolRequest, input listSensorsInput) (*mcp.CallToolResult, listSensorsOutput, error) {
	filter, err := normalizeSensorFilter(input)
	if err != nil {
		return nil, listSensorsOutput{}, err
	}
	if t.metrics == nil {
		return nil, listSensorsOutput{}, errors.New("host metrics are unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	snapshot := t.metrics.MetricsSnapshot(ctx)
	sensors := snapshot.Metrics.Sensors
	// Project into fresh slices: the snapshot slices are shared with the
	// sensor cache, the events ticker and the health report, so they must
	// never be sorted or truncated in place.
	temperatures := temperatureSensorOutputs(sensors.Temperatures)
	fans := fanSensorOutputs(sensors.Fans)
	power := powerSensorOutputs(sensors.Power)

	// Rank the whole matched population first. The rollup describes all of it
	// and is never truncated; the category lists carry only its worst `limit`.
	rankedTemperatures := rankTemperatureSensors(temperatures, filter)
	rankedFans := rankFanSensors(fans, filter)
	rankedPower := rankPowerSensors(power, filter)

	output := listSensorsOutput{
		Platform:     runtime.GOOS,
		CollectedAt:  snapshot.Metrics.CollectedAt,
		Summary:      summarizeSensors(temperatures, fans, power),
		Sources:      sensorSources(rankedTemperatures, rankedFans, rankedPower),
		Matched:      len(rankedTemperatures) + len(rankedFans) + len(rankedPower),
		Temperatures: rankedTemperatures,
		Fans:         rankedFans,
		Power:        rankedPower,
	}
	if len(output.Temperatures) > filter.limit {
		output.Temperatures = output.Temperatures[:filter.limit]
	}
	if len(output.Fans) > filter.limit {
		output.Fans = output.Fans[:filter.limit]
	}
	if len(output.Power) > filter.limit {
		output.Power = output.Power[:filter.limit]
	}
	output.Returned = len(output.Temperatures) + len(output.Fans) + len(output.Power)
	output.Truncated = output.Matched > output.Returned
	return nil, output, nil
}

func normalizeSensorFilter(input listSensorsInput) (sensorFilter, error) {
	filter := sensorFilter{
		category: strings.ToLower(strings.TrimSpace(input.Category)),
		match:    strings.ToLower(strings.TrimSpace(input.Match)),
		limit:    input.Limit,
	}
	switch filter.category {
	case "", sensorCategoryTemperature, sensorCategoryFan, sensorCategoryPower:
	default:
		return sensorFilter{}, fmt.Errorf("unsupported category %q", input.Category)
	}
	for _, status := range input.Status {
		normalized := strings.ToLower(strings.TrimSpace(status))
		switch normalized {
		case opsplane.MetricPostureSeverityOK, opsplane.MetricPostureSeverityWarning,
			opsplane.MetricPostureSeverityCritical, opsplane.MetricPostureSeverityUnknown:
		default:
			return sensorFilter{}, fmt.Errorf("unsupported status %q", status)
		}
		if !slices.Contains(filter.statuses, normalized) {
			filter.statuses = append(filter.statuses, normalized)
		}
	}
	if filter.limit <= 0 {
		filter.limit = defaultSensorLimit
	}
	if filter.limit > maxSensorLimit {
		filter.limit = maxSensorLimit
	}
	return filter, nil
}

func (f sensorFilter) includes(category string) bool {
	return f.category == "" || f.category == category
}

func (f sensorFilter) keeps(status, id, label, source string) bool {
	if len(f.statuses) > 0 && !slices.Contains(f.statuses, status) {
		return false
	}
	if f.match == "" {
		return true
	}
	return strings.Contains(strings.ToLower(id), f.match) ||
		strings.Contains(strings.ToLower(label), f.match) ||
		strings.Contains(strings.ToLower(source), f.match)
}

func temperatureSensorOutputs(sensors []opsplane.TemperatureSensor) []temperatureSensorOutput {
	result := make([]temperatureSensorOutput, 0, len(sensors))
	for _, sensor := range sensors {
		result = append(result, temperatureSensorOutput{
			ID:              sensor.ID,
			Label:           sensor.Label,
			Source:          sensor.Source,
			Status:          opsplane.TemperatureSensorStatus(sensor),
			Celsius:         sensor.Celsius,
			MaxCelsius:      sensor.MaxCelsius,
			CriticalCelsius: sensor.CriticalCelsius,
			Alarm:           sensor.Alarm,
		})
	}
	return result
}

func fanSensorOutputs(sensors []opsplane.FanSensor) []fanSensorOutput {
	result := make([]fanSensorOutput, 0, len(sensors))
	for _, sensor := range sensors {
		result = append(result, fanSensorOutput{
			ID:     sensor.ID,
			Label:  sensor.Label,
			Source: sensor.Source,
			Status: opsplane.FanSensorStatus(sensor),
			RPM:    sensor.RPM,
			MinRPM: sensor.MinRPM,
			MaxRPM: sensor.MaxRPM,
			Alarm:  sensor.Alarm,
		})
	}
	return result
}

func powerSensorOutputs(sensors []opsplane.PowerSensor) []powerSensorOutput {
	result := make([]powerSensorOutput, 0, len(sensors))
	for _, sensor := range sensors {
		result = append(result, powerSensorOutput{
			ID:            sensor.ID,
			Label:         sensor.Label,
			Source:        sensor.Source,
			Status:        opsplane.PowerSensorStatus(sensor),
			Watts:         sensor.Watts,
			MaxWatts:      sensor.MaxWatts,
			CriticalWatts: sensor.CriticalWatts,
			CapWatts:      sensor.CapWatts,
			Alarm:         sensor.Alarm,
		})
	}
	return result
}

func rankTemperatureSensors(sensors []temperatureSensorOutput, filter sensorFilter) []temperatureSensorOutput {
	result := []temperatureSensorOutput{}
	if !filter.includes(sensorCategoryTemperature) {
		return result
	}
	for _, sensor := range sensors {
		if filter.keeps(sensor.Status, sensor.ID, sensor.Label, sensor.Source) {
			result = append(result, sensor)
		}
	}
	slices.SortFunc(result, func(a, b temperatureSensorOutput) int {
		return compareSensorOrder(
			sensorOrder{status: a.Status, value: a.Celsius, id: a.ID},
			sensorOrder{status: b.Status, value: b.Celsius, id: b.ID},
		)
	})
	return result
}

func rankFanSensors(sensors []fanSensorOutput, filter sensorFilter) []fanSensorOutput {
	result := []fanSensorOutput{}
	if !filter.includes(sensorCategoryFan) {
		return result
	}
	for _, sensor := range sensors {
		if filter.keeps(sensor.Status, sensor.ID, sensor.Label, sensor.Source) {
			result = append(result, sensor)
		}
	}
	slices.SortFunc(result, func(a, b fanSensorOutput) int {
		return compareSensorOrder(
			sensorOrder{status: a.Status, value: float64(a.RPM), id: a.ID},
			sensorOrder{status: b.Status, value: float64(b.RPM), id: b.ID},
		)
	})
	return result
}

func rankPowerSensors(sensors []powerSensorOutput, filter sensorFilter) []powerSensorOutput {
	result := []powerSensorOutput{}
	if !filter.includes(sensorCategoryPower) {
		return result
	}
	for _, sensor := range sensors {
		if filter.keeps(sensor.Status, sensor.ID, sensor.Label, sensor.Source) {
			result = append(result, sensor)
		}
	}
	slices.SortFunc(result, func(a, b powerSensorOutput) int {
		return compareSensorOrder(
			sensorOrder{status: a.Status, value: a.Watts, id: a.ID},
			sensorOrder{status: b.Status, value: b.Watts, id: b.ID},
		)
	})
	return result
}

// sensorRollupRow is one sensor reduced to the fields a rollup needs, so the
// three categories share one grouping pass instead of triplicating it.
type sensorRollupRow struct {
	category string
	source   string
	status   string
	value    float64
	id       string
}

// sensorSources groups the matched sensors by category and chip. Rows follow
// the category order the response itself uses, then worst status, then source,
// so the block is stable across calls.
func sensorSources(
	temperatures []temperatureSensorOutput,
	fans []fanSensorOutput,
	power []powerSensorOutput,
) []sensorSourceOutput {
	rows := make([]sensorRollupRow, 0, len(temperatures)+len(fans)+len(power))
	for _, sensor := range temperatures {
		rows = append(rows, sensorRollupRow{sensorCategoryTemperature, sensor.Source, sensor.Status, sensor.Celsius, sensor.ID})
	}
	for _, sensor := range fans {
		rows = append(rows, sensorRollupRow{sensorCategoryFan, sensor.Source, sensor.Status, float64(sensor.RPM), sensor.ID})
	}
	for _, sensor := range power {
		rows = append(rows, sensorRollupRow{sensorCategoryPower, sensor.Source, sensor.Status, sensor.Watts, sensor.ID})
	}

	order := make([]string, 0, len(rows))
	groups := make(map[string]*sensorSourceOutput, len(rows))
	totals := make(map[string]float64, len(rows))
	finite := make(map[string]int, len(rows))
	worst := make(map[string]sensorOrder, len(rows))
	for _, row := range rows {
		key := row.category + "\x00" + row.source
		group, exists := groups[key]
		if !exists {
			group = &sensorSourceOutput{Category: row.category, Source: row.source, Status: opsplane.MetricPostureSeverityUnknown}
			groups[key] = group
			order = append(order, key)
		}
		group.Count++
		switch row.status {
		case opsplane.MetricPostureSeverityCritical:
			group.Critical++
		case opsplane.MetricPostureSeverityWarning:
			group.Warning++
		case opsplane.MetricPostureSeverityUnknown:
			group.Unknown++
		}
		candidate := sensorOrder{status: row.status, value: row.value, id: row.id}
		if current, seen := worst[key]; !seen || compareSensorOrder(candidate, current) < 0 {
			worst[key] = candidate
			group.Status = row.status
			group.WorstID = row.id
		}
		if math.IsNaN(row.value) || math.IsInf(row.value, 0) {
			continue
		}
		if finite[key] == 0 || row.value < group.Lowest {
			group.Lowest = row.value
		}
		if finite[key] == 0 || row.value > group.Highest {
			group.Highest = row.value
		}
		totals[key] += row.value
		finite[key]++
	}

	result := make([]sensorSourceOutput, 0, len(order))
	for _, key := range order {
		group := groups[key]
		if count := finite[key]; count > 0 {
			// One decimal keeps the rollup readable without implying the
			// average carries more precision than the readings behind it.
			group.Average = math.Round(totals[key]/float64(count)*10) / 10
		}
		result = append(result, *group)
	}
	slices.SortFunc(result, func(a, b sensorSourceOutput) int {
		if category := cmp.Compare(sensorCategoryRank(a.Category), sensorCategoryRank(b.Category)); category != 0 {
			return category
		}
		if rank := cmp.Compare(sensorStatusRank(b.Status), sensorStatusRank(a.Status)); rank != 0 {
			return rank
		}
		return strings.Compare(a.Source, b.Source)
	})
	return result
}

func sensorCategoryRank(category string) int {
	switch category {
	case sensorCategoryTemperature:
		return 0
	case sensorCategoryFan:
		return 1
	default:
		return 2
	}
}

// compareSensorOrder ranks worst first, then highest reading, then by the only
// unique key a sensor has. Truncation therefore always keeps the readings an
// operator would look at first.
func compareSensorOrder(a, b sensorOrder) int {
	if rank := cmp.Compare(sensorStatusRank(b.status), sensorStatusRank(a.status)); rank != 0 {
		return rank
	}
	if value := cmp.Compare(b.value, a.value); value != 0 {
		return value
	}
	return strings.Compare(a.id, b.id)
}

func sensorStatusRank(status string) int {
	switch status {
	case opsplane.MetricPostureSeverityCritical:
		return 3
	case opsplane.MetricPostureSeverityWarning:
		return 2
	case opsplane.MetricPostureSeverityOK:
		return 1
	default:
		return 0
	}
}

func summarizeSensors(
	temperatures []temperatureSensorOutput,
	fans []fanSensorOutput,
	power []powerSensorOutput,
) sensorSummaryOutput {
	summary := sensorSummaryOutput{Total: len(temperatures) + len(fans) + len(power)}
	for _, sensor := range temperatures {
		summary.count(sensor.Status)
		if !hotterSensor(sensor, summary.Hottest) {
			continue
		}
		// Copy: the projection slices are ranked and truncated afterwards.
		hottest := sensor
		summary.Hottest = &hottest
	}
	for _, sensor := range fans {
		summary.count(sensor.Status)
	}
	for _, sensor := range power {
		summary.count(sensor.Status)
	}
	return summary
}

func hotterSensor(sensor temperatureSensorOutput, best *temperatureSensorOutput) bool {
	if math.IsNaN(sensor.Celsius) || math.IsInf(sensor.Celsius, 0) {
		return false
	}
	return best == nil || sensor.Celsius > best.Celsius
}

func (s *sensorSummaryOutput) count(status string) {
	switch status {
	case opsplane.MetricPostureSeverityCritical:
		s.Critical++
	case opsplane.MetricPostureSeverityWarning:
		s.Warning++
	case opsplane.MetricPostureSeverityUnknown:
		s.Unknown++
	}
}
