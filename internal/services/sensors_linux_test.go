//go:build linux

package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLinuxSensorCollectorReadsHWMonCategories(t *testing.T) {
	root := useSensorSysRoot(t)
	hwmon := filepath.Join(root, "class", "hwmon", "hwmon0")
	writeSensorFixture(t, hwmon, "name", "coretemp\n")
	writeSensorFixture(t, hwmon, "temp1_input", "67500\n")
	writeSensorFixture(t, hwmon, "temp1_label", "Package id 0\n")
	writeSensorFixture(t, hwmon, "temp1_max", "80000\n")
	writeSensorFixture(t, hwmon, "temp1_crit", "95000\n")
	writeSensorFixture(t, hwmon, "temp1_alarm", "0\n")
	writeSensorFixture(t, hwmon, "fan1_input", "1320\n")
	writeSensorFixture(t, hwmon, "fan1_min", "500\n")
	writeSensorFixture(t, hwmon, "fan1_alarm", "1\n")
	writeSensorFixture(t, hwmon, "power1_input", "45000000\n")
	writeSensorFixture(t, hwmon, "power1_average", "44000000\n")
	writeSensorFixture(t, hwmon, "power1_label", "Package\n")
	writeSensorFixture(t, hwmon, "power1_cap", "65000000\n")

	got := newSensorCollector()(time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC))
	if len(got.Temperatures) != 1 || len(got.Fans) != 1 || len(got.Power) != 1 {
		t.Fatalf("sensor categories = %+v", got)
	}
	temperature := got.Temperatures[0]
	if temperature.ID != "hwmon0:temp1" || temperature.Label != "Package id 0" ||
		temperature.Source != "coretemp" || temperature.Celsius != 67.5 ||
		temperature.MaxCelsius == nil || *temperature.MaxCelsius != 80 ||
		temperature.CriticalCelsius == nil || *temperature.CriticalCelsius != 95 ||
		temperature.Alarm == nil || *temperature.Alarm {
		t.Fatalf("temperature = %+v", temperature)
	}
	fan := got.Fans[0]
	if fan.RPM != 1320 || fan.MinRPM == nil || *fan.MinRPM != 500 ||
		fan.Alarm == nil || !*fan.Alarm {
		t.Fatalf("fan = %+v", fan)
	}
	power := got.Power[0]
	if power.Watts != 45 || power.CapWatts == nil || *power.CapWatts != 65 {
		t.Fatalf("power = %+v", power)
	}
}

func TestLinuxSensorCollectorFallsBackToThermalZones(t *testing.T) {
	root := useSensorSysRoot(t)
	zone := filepath.Join(root, "class", "thermal", "thermal_zone0")
	writeSensorFixture(t, zone, "type", "x86_pkg_temp\n")
	writeSensorFixture(t, zone, "temp", "42000\n")
	writeSensorFixture(t, zone, "trip_point_0_type", "critical\n")
	writeSensorFixture(t, zone, "trip_point_0_temp", "100000\n")

	got := newSensorCollector()(time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC))
	if len(got.Temperatures) != 1 {
		t.Fatalf("temperatures = %+v", got.Temperatures)
	}
	sensor := got.Temperatures[0]
	if sensor.ID != "thermal:thermal_zone0" || sensor.Label != "x86_pkg_temp" ||
		sensor.Celsius != 42 || sensor.CriticalCelsius == nil || *sensor.CriticalCelsius != 100 {
		t.Fatalf("thermal sensor = %+v", sensor)
	}
}

func TestLinuxSensorCollectorDerivesPowerFromEnergyDeltaAndRollover(t *testing.T) {
	root := useSensorSysRoot(t)
	zone := filepath.Join(root, "class", "powercap", "intel-rapl:0")
	writeSensorFixture(t, zone, "name", "package-0\n")
	writeSensorFixture(t, zone, "energy_uj", "9500000\n")
	writeSensorFixture(t, zone, "max_energy_range_uj", "10000000\n")
	writeSensorFixture(t, zone, "constraint_0_power_limit_uw", "15000000\n")

	collector := newSensorCollector()
	start := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	if first := collector(start); len(first.Power) != 0 {
		t.Fatalf("first power sample = %+v, want no invented rate", first.Power)
	}
	writeSensorFixture(t, zone, "energy_uj", "500000\n")
	second := collector(start.Add(10 * time.Second))
	if len(second.Power) != 1 {
		t.Fatalf("second power sample = %+v", second.Power)
	}
	power := second.Power[0]
	if power.ID != "powercap:intel-rapl:0" || power.Label != "package-0" ||
		power.Watts != 0.1 || power.CapWatts == nil || *power.CapWatts != 15 {
		t.Fatalf("derived power = %+v", power)
	}
}

func TestLinuxSensorCollectorIgnoresMalformedAndMissingSources(t *testing.T) {
	root := useSensorSysRoot(t)
	hwmon := filepath.Join(root, "class", "hwmon", "hwmon0")
	writeSensorFixture(t, hwmon, "name", "broken\n")
	writeSensorFixture(t, hwmon, "temp1_input", "invalid\n")
	writeSensorFixture(t, hwmon, "fan1_input", "-1\n")
	writeSensorFixture(t, hwmon, "power1_input", "invalid\n")

	got := newSensorCollector()(time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC))
	if got.Temperatures == nil || got.Fans == nil || got.Power == nil {
		t.Fatalf("empty categories must be arrays: %+v", got)
	}
	if len(got.Temperatures) != 0 || len(got.Fans) != 0 || len(got.Power) != 0 {
		t.Fatalf("malformed sensors were not ignored: %+v", got)
	}
}

func TestLinuxSensorCollectorDropsImpossibleTemperatureLimits(t *testing.T) {
	root := useSensorSysRoot(t)
	hwmon := filepath.Join(root, "class", "hwmon", "hwmon0")
	writeSensorFixture(t, hwmon, "name", "nvme\n")
	writeSensorFixture(t, hwmon, "temp1_input", "43850\n")
	writeSensorFixture(t, hwmon, "temp1_max", "65261850\n")
	writeSensorFixture(t, hwmon, "temp1_crit", "87850\n")

	got := newSensorCollector()(time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC))
	if len(got.Temperatures) != 1 {
		t.Fatalf("temperatures = %+v", got.Temperatures)
	}
	sensor := got.Temperatures[0]
	if sensor.MaxCelsius != nil || sensor.CriticalCelsius == nil || *sensor.CriticalCelsius != 87.85 {
		t.Fatalf("temperature thresholds = %+v", sensor)
	}
}

func useSensorSysRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	original := sysRootPath
	t.Cleanup(func() { sysRootPath = original })
	sysRootPath = root
	return root
}

func writeSensorFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLinuxSensorCollectorCountsEachPowercapZoneOnce(t *testing.T) {
	root := useSensorSysRoot(t)
	class := filepath.Join(root, "class", "powercap")
	// Sysfs registers every zone under its controller and links each one into
	// the class directory, so one physical zone is reachable by several paths.
	zone := filepath.Join(root, "devices", "virtual", "powercap", "intel-rapl", "intel-rapl:0")
	sub := filepath.Join(zone, "intel-rapl:0:0")
	writeSensorFixture(t, zone, "name", "package-0\n")
	writeSensorFixture(t, zone, "energy_uj", "1000000\n")
	writeSensorFixture(t, sub, "name", "core\n")
	writeSensorFixture(t, sub, "energy_uj", "400000\n")

	if err := os.MkdirAll(class, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	for link, target := range map[string]string{
		filepath.Join(class, "intel-rapl"):     filepath.Dir(zone),
		filepath.Join(class, "intel-rapl:0"):   zone,
		filepath.Join(class, "intel-rapl:0:0"): sub,
	} {
		if err := os.Symlink(target, link); err != nil {
			t.Fatalf("Symlink(%s) error = %v", link, err)
		}
	}
	// The kernel also exposes each sub-zone's parent as "device".
	if err := os.Symlink(zone, filepath.Join(sub, "device")); err != nil {
		t.Fatalf("Symlink(device) error = %v", err)
	}

	collector := newSensorCollector()
	start := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	collector(start)
	writeSensorFixture(t, zone, "energy_uj", "3000000\n")
	writeSensorFixture(t, sub, "energy_uj", "1400000\n")
	got := collector(start.Add(10 * time.Second))

	if len(got.Power) != 2 {
		t.Fatalf("power = %+v, want one reading per physical zone", got.Power)
	}
	// Readings are label-ordered, so "core" precedes "package-0".
	wantIDs := []string{"powercap:intel-rapl:0:0", "powercap:intel-rapl:0"}
	for index, want := range wantIDs {
		if got.Power[index].ID != want {
			t.Fatalf("power[%d].ID = %q, want %q", index, got.Power[index].ID, want)
		}
	}
	if got.Power[0].Watts != 0.1 || got.Power[1].Watts != 0.2 {
		t.Fatalf("watts = %+v", got.Power)
	}
}
