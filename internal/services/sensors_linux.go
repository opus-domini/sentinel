//go:build linux

package services

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var sysRootPath = "/sys"

type energyReading struct {
	microjoules uint64
	maxRange    uint64
	at          time.Time
}

type linuxSensorCollector struct {
	previousEnergy map[string]energyReading
}

func newSensorCollector() func(time.Time) SensorMetrics {
	collector := &linuxSensorCollector{
		previousEnergy: make(map[string]energyReading),
	}
	return collector.collect
}

func (c *linuxSensorCollector) collect(now time.Time) SensorMetrics {
	metrics := emptySensorMetrics()
	c.collectHWMon(&metrics)
	if len(metrics.Temperatures) == 0 {
		c.collectThermalZones(&metrics)
	}
	c.collectPowercap(now, &metrics)
	sortSensorMetrics(&metrics)
	return metrics
}

func (c *linuxSensorCollector) collectHWMon(metrics *SensorMetrics) {
	root := filepath.Join(sysRootPath, "class", "hwmon")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "hwmon") {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		chip := readText(filepath.Join(dir, "name"))
		if chip == "" {
			chip = entry.Name()
		}
		files, readErr := os.ReadDir(dir)
		if readErr != nil {
			continue
		}

		powerFeatures := make(map[string]string)
		for _, file := range files {
			name := file.Name()
			switch {
			case isSensorInput(name, "temp"):
				feature := strings.TrimSuffix(name, "_input")
				if sensor, ok := readTemperatureSensor(dir, entry.Name(), chip, feature); ok {
					metrics.Temperatures = append(metrics.Temperatures, sensor)
				}
			case isSensorInput(name, "fan"):
				feature := strings.TrimSuffix(name, "_input")
				if sensor, ok := readFanSensor(dir, entry.Name(), chip, feature); ok {
					metrics.Fans = append(metrics.Fans, sensor)
				}
			case isSensorInput(name, "power"):
				feature := strings.TrimSuffix(name, "_input")
				powerFeatures[feature] = "input"
			case isSensorAverage(name, "power"):
				feature := strings.TrimSuffix(name, "_average")
				if _, exists := powerFeatures[feature]; !exists {
					powerFeatures[feature] = "average"
				}
			}
		}
		for feature, reading := range powerFeatures {
			if sensor, ok := readPowerSensor(dir, entry.Name(), chip, feature, reading); ok {
				metrics.Power = append(metrics.Power, sensor)
			}
		}
	}
}

func (c *linuxSensorCollector) collectThermalZones(metrics *SensorMetrics) {
	root := filepath.Join(sysRootPath, "class", "thermal")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "thermal_zone") {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		current, ok := readTemperatureCelsius(filepath.Join(dir, "temp"))
		if !ok {
			continue
		}
		label := readText(filepath.Join(dir, "type"))
		if label == "" {
			label = entry.Name()
		}
		metrics.Temperatures = append(metrics.Temperatures, TemperatureSensor{
			ID:              "thermal:" + entry.Name(),
			Label:           label,
			Source:          "thermal",
			Celsius:         current,
			CriticalCelsius: readThermalCritical(dir),
		})
	}
}

func (c *linuxSensorCollector) collectPowercap(now time.Time, metrics *SensorMetrics) {
	paths := powercapEnergyPaths()
	next := make(map[string]energyReading, len(paths))
	for _, energyPath := range paths {
		dir := filepath.Dir(energyPath)
		rawEnergy, ok := readUint64(energyPath)
		if !ok {
			continue
		}
		maxRange, _ := readUint64(filepath.Join(dir, "max_energy_range_uj"))
		id := powercapSensorID(dir)
		current := energyReading{microjoules: rawEnergy, maxRange: maxRange, at: now}
		next[id] = current

		previous, exists := c.previousEnergy[id]
		if !exists || !now.After(previous.at) {
			continue
		}
		delta, valid := energyDelta(previous, current)
		if !valid {
			continue
		}
		seconds := now.Sub(previous.at).Seconds()
		watts := float64(delta) / 1_000_000 / seconds
		label := readText(filepath.Join(dir, "name"))
		if label == "" {
			label = filepath.Base(dir)
		}
		metrics.Power = append(metrics.Power, PowerSensor{
			ID:       id,
			Label:    label,
			Source:   "powercap",
			Watts:    watts,
			CapWatts: readMicrowattsPtr(filepath.Join(dir, "constraint_0_power_limit_uw")),
		})
	}
	c.previousEnergy = next
}

func readTemperatureSensor(dir, hwmon, chip, feature string) (TemperatureSensor, bool) {
	current, ok := readTemperatureCelsius(filepath.Join(dir, feature+"_input"))
	if !ok {
		return TemperatureSensor{}, false
	}
	return TemperatureSensor{
		ID:              hwmon + ":" + feature,
		Label:           sensorLabel(dir, chip, feature),
		Source:          chip,
		Celsius:         current,
		MaxCelsius:      readTemperatureCelsiusPtr(filepath.Join(dir, feature+"_max")),
		CriticalCelsius: readTemperatureCelsiusPtr(filepath.Join(dir, feature+"_crit")),
		Alarm:           readBoolPtr(filepath.Join(dir, feature+"_alarm")),
	}, true
}

func readFanSensor(dir, hwmon, chip, feature string) (FanSensor, bool) {
	rpm, ok := readInt64(filepath.Join(dir, feature+"_input"))
	if !ok || rpm < 0 {
		return FanSensor{}, false
	}
	return FanSensor{
		ID:     hwmon + ":" + feature,
		Label:  sensorLabel(dir, chip, feature),
		Source: chip,
		RPM:    rpm,
		MinRPM: readInt64Ptr(filepath.Join(dir, feature+"_min")),
		MaxRPM: readInt64Ptr(filepath.Join(dir, feature+"_max")),
		Alarm:  readBoolPtr(filepath.Join(dir, feature+"_alarm")),
	}, true
}

func readPowerSensor(dir, hwmon, chip, feature, reading string) (PowerSensor, bool) {
	watts, ok := readScaledFloat(filepath.Join(dir, feature+"_"+reading), 1_000_000)
	if !ok || watts < 0 {
		return PowerSensor{}, false
	}
	return PowerSensor{
		ID:            hwmon + ":" + feature,
		Label:         sensorLabel(dir, chip, feature),
		Source:        chip,
		Watts:         watts,
		MaxWatts:      readMicrowattsPtr(filepath.Join(dir, feature+"_max")),
		CriticalWatts: readMicrowattsPtr(filepath.Join(dir, feature+"_crit")),
		CapWatts:      readMicrowattsPtr(filepath.Join(dir, feature+"_cap")),
		Alarm:         readBoolPtr(filepath.Join(dir, feature+"_alarm")),
	}, true
}

func readThermalCritical(dir string) *float64 {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, file := range files {
		name := file.Name()
		if !strings.HasPrefix(name, "trip_point_") || !strings.HasSuffix(name, "_type") {
			continue
		}
		kind := strings.ToLower(readText(filepath.Join(dir, name)))
		if kind != "critical" && kind != "hot" {
			continue
		}
		stem := strings.TrimSuffix(name, "_type")
		if value := readTemperatureCelsiusPtr(filepath.Join(dir, stem+"_temp")); value != nil {
			return value
		}
	}
	return nil
}

// powercapEnergyPaths lists one energy counter per physical powercap zone.
// Every zone, nested sub-zones included, is linked directly under
// /sys/class/powercap, and each is reachable again through its parent zone and
// through a "device" symlink. Those aliases are the same file, so zones are
// deduplicated by resolved path. The shallowest alias wins, which keeps the
// zone's canonical sysfs name in the sensor ID powercapSensorID derives.
func powercapEnergyPaths() []string {
	root := filepath.Join(sysRootPath, "class", "powercap")
	matches := make([]string, 0)
	for _, pattern := range []string{
		filepath.Join(root, "*", "energy_uj"),
		filepath.Join(root, "*", "*", "energy_uj"),
	} {
		found, _ := filepath.Glob(pattern)
		matches = append(matches, found...)
	}
	sort.Slice(matches, func(i, j int) bool {
		left := strings.Count(matches[i], string(filepath.Separator))
		right := strings.Count(matches[j], string(filepath.Separator))
		if left != right {
			return left < right
		}
		return matches[i] < matches[j]
	})
	seen := make(map[string]struct{}, len(matches))
	paths := make([]string, 0, len(matches))
	for _, match := range matches {
		zone, err := filepath.EvalSymlinks(match)
		if err != nil {
			zone = match
		}
		if _, exists := seen[zone]; exists {
			continue
		}
		seen[zone] = struct{}{}
		paths = append(paths, match)
	}
	sort.Strings(paths)
	return paths
}

func powercapSensorID(dir string) string {
	root := filepath.Join(sysRootPath, "class", "powercap")
	relative, err := filepath.Rel(root, dir)
	if err != nil {
		relative = filepath.Base(dir)
	}
	return "powercap:" + strings.ReplaceAll(filepath.ToSlash(relative), "/", ":")
}

func energyDelta(previous, current energyReading) (uint64, bool) {
	if current.microjoules >= previous.microjoules {
		return current.microjoules - previous.microjoules, true
	}
	maxRange := current.maxRange
	if maxRange == 0 {
		maxRange = previous.maxRange
	}
	if maxRange == 0 || previous.microjoules > maxRange {
		return 0, false
	}
	return maxRange - previous.microjoules + current.microjoules, true
}

func sensorLabel(dir, chip, feature string) string {
	if label := readText(filepath.Join(dir, feature+"_label")); label != "" {
		return label
	}
	if chip != "" {
		return chip + " " + feature
	}
	return feature
}

func isSensorInput(name, prefix string) bool {
	return sensorFeature(name, prefix, "_input")
}

func isSensorAverage(name, prefix string) bool {
	return sensorFeature(name, prefix, "_average")
}

func sensorFeature(name, prefix, suffix string) bool {
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return false
	}
	index := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
	if index == "" {
		return false
	}
	for _, char := range index {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func readText(path string) string {
	// #nosec G304 -- callers only pass paths discovered beneath the kernel's sysfs tree.
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readInt64(path string) (int64, bool) {
	value, err := strconv.ParseInt(readText(path), 10, 64)
	return value, err == nil
}

func readUint64(path string) (uint64, bool) {
	value, err := strconv.ParseUint(readText(path), 10, 64)
	return value, err == nil
}

func readScaledFloat(path string, scale float64) (float64, bool) {
	value, ok := readInt64(path)
	if !ok || scale == 0 {
		return 0, false
	}
	return float64(value) / scale, true
}

func readMicrowattsPtr(path string) *float64 {
	value, ok := readScaledFloat(path, 1_000_000)
	if !ok {
		return nil
	}
	return &value
}

func readTemperatureCelsius(path string) (float64, bool) {
	value, ok := readScaledFloat(path, 1_000)
	if !ok || value < -273.15 || value > 1_000 {
		return 0, false
	}
	return value, true
}

func readTemperatureCelsiusPtr(path string) *float64 {
	value, ok := readTemperatureCelsius(path)
	if !ok {
		return nil
	}
	return &value
}

func readInt64Ptr(path string) *int64 {
	value, ok := readInt64(path)
	if !ok {
		return nil
	}
	return &value
}

func readBoolPtr(path string) *bool {
	value, ok := readInt64(path)
	if !ok {
		return nil
	}
	result := value != 0
	return &result
}

func sortSensorMetrics(metrics *SensorMetrics) {
	sort.Slice(metrics.Temperatures, func(i, j int) bool {
		return sensorLess(
			metrics.Temperatures[i].Label,
			metrics.Temperatures[i].Source,
			metrics.Temperatures[i].ID,
			metrics.Temperatures[j].Label,
			metrics.Temperatures[j].Source,
			metrics.Temperatures[j].ID,
		)
	})
	sort.Slice(metrics.Fans, func(i, j int) bool {
		return sensorLess(
			metrics.Fans[i].Label,
			metrics.Fans[i].Source,
			metrics.Fans[i].ID,
			metrics.Fans[j].Label,
			metrics.Fans[j].Source,
			metrics.Fans[j].ID,
		)
	})
	sort.Slice(metrics.Power, func(i, j int) bool {
		return sensorLess(
			metrics.Power[i].Label,
			metrics.Power[i].Source,
			metrics.Power[i].ID,
			metrics.Power[j].Label,
			metrics.Power[j].Source,
			metrics.Power[j].ID,
		)
	})
}

func sensorLess(leftLabel, leftSource, leftID, rightLabel, rightSource, rightID string) bool {
	left := strings.ToLower(leftLabel + "\x00" + leftSource + "\x00" + leftID)
	right := strings.ToLower(rightLabel + "\x00" + rightSource + "\x00" + rightID)
	return left < right
}
