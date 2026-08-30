package mcpserver

import (
	"context"
	"encoding/json"
	"math"
	"runtime"
	"strings"
	"testing"

	opsplane "github.com/opus-domini/sentinel/internal/services"
)

type fakeMetricsSource struct {
	snapshot opsplane.MetricsSnapshot
	calls    int
}

func (f *fakeMetricsSource) MetricsSnapshot(context.Context) opsplane.MetricsSnapshot {
	f.calls++
	return f.snapshot
}

var _ metricsSource = (*fakeMetricsSource)(nil)

func sensorFloatPtr(value float64) *float64 { return &value }

// sensorFixture mirrors a real host: many classifiable temperatures, fans with
// no alarm file at all, and a powercap reading with no limits.
func sensorFixture() opsplane.MetricsSnapshot {
	return opsplane.MetricsSnapshot{
		Metrics: opsplane.HostMetrics{
			CollectedAt: "2026-08-30T12:00:00Z",
			Sensors: opsplane.SensorMetrics{
				Temperatures: []opsplane.TemperatureSensor{
					{ID: "hwmon5:temp1", Label: "Composite", Source: "nvme", Celsius: 40, MaxCelsius: sensorFloatPtr(82), CriticalCelsius: sensorFloatPtr(85), Alarm: boolPtr(false)},
					{ID: "hwmon9:temp1", Label: "Package id 0", Source: "coretemp", Celsius: 92, MaxCelsius: sensorFloatPtr(80), CriticalCelsius: sensorFloatPtr(95)},
					{ID: "hwmon9:temp2", Label: "Core 0", Source: "coretemp", Celsius: 45, MaxCelsius: sensorFloatPtr(80), CriticalCelsius: sensorFloatPtr(95)},
					{ID: "hwmon3:temp1", Label: "acpitz temp1", Source: "acpitz", Celsius: 101},
					{ID: "hwmon7:temp1", Label: "spd5118 temp1", Source: "spd5118", Celsius: 100, MaxCelsius: sensorFloatPtr(80), CriticalCelsius: sensorFloatPtr(95)},
				},
				Fans: []opsplane.FanSensor{
					{ID: "hwmon1:fan1", Label: "acpi_fan fan1", Source: "acpi_fan", RPM: 0},
					{ID: "hwmon4:fan1", Label: "CPU fan", Source: "nct6798", RPM: 3200, Alarm: boolPtr(true)},
					{ID: "hwmon4:fan2", Label: "Chassis fan", Source: "nct6798", RPM: 900, Alarm: boolPtr(false)},
				},
				Power: []opsplane.PowerSensor{
					{ID: "powercap:intel-rapl:0", Label: "package-0", Source: "powercap", Watts: 42, CapWatts: sensorFloatPtr(60)},
					{ID: "hwmon4:power1", Label: "power1", Source: "nct6798", Watts: 30, MaxWatts: sensorFloatPtr(25)},
				},
			},
		},
	}
}

func newSensorToolset() (*tools, *fakeMetricsSource) {
	source := &fakeMetricsSource{snapshot: sensorFixture()}
	return &tools{metrics: source}, source
}

func temperatureIDs(sensors []temperatureSensorOutput) []string {
	ids := make([]string, 0, len(sensors))
	for _, sensor := range sensors {
		ids = append(ids, sensor.ID)
	}
	return ids
}

func sensorIDs(output listSensorsOutput) []string {
	ids := temperatureIDs(output.Temperatures)
	for _, sensor := range output.Fans {
		ids = append(ids, sensor.ID)
	}
	for _, sensor := range output.Power {
		ids = append(ids, sensor.ID)
	}
	return ids
}

func TestListSensorsRanksWorstFirstAndSummarizesHost(t *testing.T) {
	t.Parallel()
	toolset, _ := newSensorToolset()

	_, output, err := toolset.listSensors(context.Background(), nil, listSensorsInput{})
	if err != nil {
		t.Fatalf("listSensors() error = %v", err)
	}
	if output.Platform != runtime.GOOS {
		t.Fatalf("platform = %q, want %q", output.Platform, runtime.GOOS)
	}
	if output.CollectedAt != "2026-08-30T12:00:00Z" {
		t.Fatalf("collectedAt = %q", output.CollectedAt)
	}
	wantTemperatures := []string{"hwmon7:temp1", "hwmon9:temp1", "hwmon9:temp2", "hwmon5:temp1", "hwmon3:temp1"}
	if got := temperatureIDs(output.Temperatures); !equalStrings(got, wantTemperatures) {
		t.Fatalf("temperature order = %q, want %q", got, wantTemperatures)
	}
	if output.Temperatures[0].Status != opsplane.MetricPostureSeverityCritical ||
		output.Temperatures[1].Status != opsplane.MetricPostureSeverityWarning ||
		output.Temperatures[4].Status != opsplane.MetricPostureSeverityUnknown {
		t.Fatalf("temperature statuses = %#v", output.Temperatures)
	}
	if output.Fans[0].ID != "hwmon4:fan1" || output.Fans[0].Status != opsplane.MetricPostureSeverityCritical {
		t.Fatalf("fan order = %#v", output.Fans)
	}
	wantSummary := sensorSummaryOutput{Total: 10, Warning: 2, Critical: 2, Unknown: 3}
	got := output.Summary
	hottest := got.Hottest
	got.Hottest = nil
	if got != wantSummary {
		t.Fatalf("summary = %#v, want %#v", got, wantSummary)
	}
	// The hottest sensor publishes no limits, so it is unclassifiable and
	// ranks last; the summary is the only place it always surfaces.
	if hottest == nil || hottest.ID != "hwmon3:temp1" || hottest.Celsius != 101 {
		t.Fatalf("hottest = %#v", hottest)
	}
	if output.Matched != 10 || output.Returned != 10 || output.Truncated {
		t.Fatalf("matched = %d, returned = %d, truncated = %v", output.Matched, output.Returned, output.Truncated)
	}
}

func TestListSensorsFilters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   listSensorsInput
		wantIDs []string
	}{
		{
			name:    "category keeps one group",
			input:   listSensorsInput{Category: "fan"},
			wantIDs: []string{"hwmon4:fan1", "hwmon4:fan2", "hwmon1:fan1"},
		},
		{
			name:    "category is trimmed and lowercased",
			input:   listSensorsInput{Category: "  Power  "},
			wantIDs: []string{"hwmon4:power1", "powercap:intel-rapl:0"},
		},
		{
			name:    "status set keeps warning and critical",
			input:   listSensorsInput{Status: []string{"critical", "warning"}},
			wantIDs: []string{"hwmon7:temp1", "hwmon9:temp1", "hwmon4:fan1", "hwmon4:power1"},
		},
		{
			name:    "status set can select the unjudgeable sensors",
			input:   listSensorsInput{Status: []string{"unknown"}},
			wantIDs: []string{"hwmon3:temp1", "hwmon1:fan1", "powercap:intel-rapl:0"},
		},
		{
			name:    "match hits the source",
			input:   listSensorsInput{Match: "CORETEMP"},
			wantIDs: []string{"hwmon9:temp1", "hwmon9:temp2"},
		},
		{
			name:    "match hits the label",
			input:   listSensorsInput{Match: "composite"},
			wantIDs: []string{"hwmon5:temp1"},
		},
		{
			name:    "match hits the opaque id",
			input:   listSensorsInput{Match: "powercap:intel-rapl:0"},
			wantIDs: []string{"powercap:intel-rapl:0"},
		},
		{
			name:    "filters compose with and",
			input:   listSensorsInput{Category: "temperature", Status: []string{"critical"}, Match: "spd5118"},
			wantIDs: []string{"hwmon7:temp1"},
		},
		{
			name:    "no match returns nothing",
			input:   listSensorsInput{Match: "nope"},
			wantIDs: []string{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			toolset, _ := newSensorToolset()
			_, output, err := toolset.listSensors(context.Background(), nil, test.input)
			if err != nil {
				t.Fatalf("listSensors() error = %v", err)
			}
			if got := sensorIDs(output); !equalStrings(got, test.wantIDs) {
				t.Fatalf("ids = %q, want %q", got, test.wantIDs)
			}
			if output.Matched != len(test.wantIDs) {
				t.Fatalf("matched = %d, want %d", output.Matched, len(test.wantIDs))
			}
			// The summary always describes the host, never the filtered view.
			if output.Summary.Total != 10 || output.Summary.Critical != 2 {
				t.Fatalf("summary = %#v", output.Summary)
			}
			if output.Temperatures == nil || output.Fans == nil || output.Power == nil {
				t.Fatalf("nil category slice in %#v", output)
			}
		})
	}
}

func TestListSensorsLimitsEachCategoryWorstFirst(t *testing.T) {
	t.Parallel()
	toolset, _ := newSensorToolset()

	_, output, err := toolset.listSensors(context.Background(), nil, listSensorsInput{Limit: 1})
	if err != nil {
		t.Fatalf("listSensors() error = %v", err)
	}
	wantIDs := []string{"hwmon7:temp1", "hwmon4:fan1", "hwmon4:power1"}
	if got := sensorIDs(output); !equalStrings(got, wantIDs) {
		t.Fatalf("ids = %q, want %q", got, wantIDs)
	}
	if output.Matched != 10 || output.Returned != 3 || !output.Truncated {
		t.Fatalf("matched = %d, returned = %d, truncated = %v", output.Matched, output.Returned, output.Truncated)
	}
}

func TestNormalizeSensorFilterDefaultsAndCaps(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input listSensorsInput
		want  int
	}{
		{name: "omitted", input: listSensorsInput{}, want: defaultSensorLimit},
		{name: "zero", input: listSensorsInput{Limit: 0}, want: defaultSensorLimit},
		{name: "negative", input: listSensorsInput{Limit: -5}, want: defaultSensorLimit},
		{name: "explicit", input: listSensorsInput{Limit: 7}, want: 7},
		{name: "capped", input: listSensorsInput{Limit: 5000}, want: maxSensorLimit},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			filter, err := normalizeSensorFilter(test.input)
			if err != nil {
				t.Fatalf("normalizeSensorFilter() error = %v", err)
			}
			if filter.limit != test.want {
				t.Fatalf("limit = %d, want %d", filter.limit, test.want)
			}
		})
	}
}

func TestListSensorsRejectsUnsupportedFilters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   listSensorsInput
		wantErr string
	}{
		{
			name:    "unknown category",
			input:   listSensorsInput{Category: "gpu"},
			wantErr: `unsupported category "gpu"`,
		},
		{
			name:    "plural category",
			input:   listSensorsInput{Category: "temperatures"},
			wantErr: `unsupported category "temperatures"`,
		},
		{
			name:    "frontend severity spelling",
			input:   listSensorsInput{Status: []string{"warn"}},
			wantErr: `unsupported status "warn"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			toolset, source := newSensorToolset()
			_, output, err := toolset.listSensors(context.Background(), nil, test.input)
			if err == nil || err.Error() != test.wantErr {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
			if output.Platform != "" || output.Matched != 0 {
				t.Fatalf("output = %#v, want zero value", output)
			}
			// Validation must run before any host collection.
			if source.calls != 0 {
				t.Fatalf("MetricsSnapshot calls = %d, want 0", source.calls)
			}
		})
	}
}

func TestListSensorsWithoutMetricsSource(t *testing.T) {
	t.Parallel()
	toolset := &tools{}

	_, output, err := toolset.listSensors(context.Background(), nil, listSensorsInput{})
	if err == nil || err.Error() != "host metrics are unavailable" {
		t.Fatalf("error = %v", err)
	}
	if output.Platform != "" {
		t.Fatalf("output = %#v, want zero value", output)
	}
}

func TestListSensorsEncodesEmptyCategoriesAsArrays(t *testing.T) {
	t.Parallel()
	source := &fakeMetricsSource{}
	toolset := &tools{metrics: source}

	_, output, err := toolset.listSensors(context.Background(), nil, listSensorsInput{})
	if err != nil {
		t.Fatalf("listSensors() error = %v", err)
	}
	if output.Summary.Total != 0 || output.Summary.Hottest != nil || output.Matched != 0 || output.Truncated {
		t.Fatalf("output = %#v", output)
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	for _, want := range []string{`"sources":[]`, `"temperatures":[]`, `"fans":[]`, `"power":[]`} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("payload %s missing %s", encoded, want)
		}
	}
}

func TestListSensorsDoesNotMutateTheSharedSnapshot(t *testing.T) {
	t.Parallel()
	toolset, source := newSensorToolset()
	before := temperatureIDs(temperatureSensorOutputs(source.snapshot.Metrics.Sensors.Temperatures))

	if _, _, err := toolset.listSensors(context.Background(), nil, listSensorsInput{Limit: 1}); err != nil {
		t.Fatalf("listSensors() error = %v", err)
	}
	after := temperatureIDs(temperatureSensorOutputs(source.snapshot.Metrics.Sensors.Temperatures))
	if !equalStrings(before, after) {
		t.Fatalf("cached sensor order = %q, want %q", after, before)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func TestListSensorsRollsUpEverySourceAndNeverTruncatesIt(t *testing.T) {
	t.Parallel()
	toolset, _ := newSensorToolset()

	// A limit of 1 keeps one sensor per category; the rollup still describes
	// every chip, which is what stops an agent concluding a population is absent.
	_, output, err := toolset.listSensors(context.Background(), nil, listSensorsInput{Limit: 1})
	if err != nil {
		t.Fatalf("listSensors() error = %v", err)
	}
	if output.Returned != 3 || !output.Truncated {
		t.Fatalf("returned = %d, truncated = %v", output.Returned, output.Truncated)
	}
	want := []sensorSourceOutput{
		{Category: "temperature", Source: "spd5118", Count: 1, Status: "critical", Critical: 1, Lowest: 100, Average: 100, Highest: 100, WorstID: "hwmon7:temp1"},
		{Category: "temperature", Source: "coretemp", Count: 2, Status: "warning", Warning: 1, Lowest: 45, Average: 68.5, Highest: 92, WorstID: "hwmon9:temp1"},
		{Category: "temperature", Source: "nvme", Count: 1, Status: "ok", Lowest: 40, Average: 40, Highest: 40, WorstID: "hwmon5:temp1"},
		{Category: "temperature", Source: "acpitz", Count: 1, Status: "unknown", Unknown: 1, Lowest: 101, Average: 101, Highest: 101, WorstID: "hwmon3:temp1"},
		{Category: "fan", Source: "nct6798", Count: 2, Status: "critical", Critical: 1, Lowest: 900, Average: 2050, Highest: 3200, WorstID: "hwmon4:fan1"},
		{Category: "fan", Source: "acpi_fan", Count: 1, Status: "unknown", Unknown: 1, WorstID: "hwmon1:fan1"},
		{Category: "power", Source: "nct6798", Count: 1, Status: "warning", Warning: 1, Lowest: 30, Average: 30, Highest: 30, WorstID: "hwmon4:power1"},
		{Category: "power", Source: "powercap", Count: 1, Status: "unknown", Unknown: 1, Lowest: 42, Average: 42, Highest: 42, WorstID: "powercap:intel-rapl:0"},
	}
	if len(output.Sources) != len(want) {
		t.Fatalf("sources = %#v, want %d rows", output.Sources, len(want))
	}
	for index, row := range want {
		if output.Sources[index] != row {
			t.Fatalf("sources[%d] = %#v, want %#v", index, output.Sources[index], row)
		}
	}
	// Every sensor is accounted for by the rollup even though 7 were dropped.
	counted := 0
	for _, row := range output.Sources {
		counted += row.Count
	}
	if counted != output.Matched {
		t.Fatalf("rollup counted %d sensors, matched = %d", counted, output.Matched)
	}
}

func TestListSensorsRollupFollowsTheFilter(t *testing.T) {
	t.Parallel()
	toolset, _ := newSensorToolset()

	_, output, err := toolset.listSensors(context.Background(), nil, listSensorsInput{Category: "fan"})
	if err != nil {
		t.Fatalf("listSensors() error = %v", err)
	}
	for _, row := range output.Sources {
		if row.Category != sensorCategoryFan {
			t.Fatalf("sources = %#v, want fans only", output.Sources)
		}
	}
	if len(output.Sources) != 2 {
		t.Fatalf("sources = %#v, want one row per fan chip", output.Sources)
	}
}

func TestSensorSourcesIgnoresUnreadableReadings(t *testing.T) {
	t.Parallel()
	sources := sensorSources([]temperatureSensorOutput{
		{ID: "a", Source: "chip", Status: "ok", Celsius: math.NaN()},
		{ID: "b", Source: "chip", Status: "ok", Celsius: 50},
		{ID: "c", Source: "chip", Status: "ok", Celsius: 70},
	}, nil, nil)

	if len(sources) != 1 {
		t.Fatalf("sources = %#v", sources)
	}
	row := sources[0]
	// The unreadable sensor is counted but never skews the range or average.
	if row.Count != 3 || row.Lowest != 50 || row.Highest != 70 || row.Average != 60 {
		t.Fatalf("row = %#v", row)
	}
}
