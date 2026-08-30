package services

import (
	"math"
	"testing"
)

func sensorFloatPtr(value float64) *float64 { return &value }

func sensorBoolPtr(value bool) *bool { return &value }

func sensorInt64Ptr(value int64) *int64 { return &value }

func TestTemperatureSensorStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		sensor TemperatureSensor
		want   string
	}{
		{
			name:   "alarm outranks limits",
			sensor: TemperatureSensor{Celsius: 40, MaxCelsius: sensorFloatPtr(80), CriticalCelsius: sensorFloatPtr(95), Alarm: sensorBoolPtr(true)},
			want:   MetricPostureSeverityCritical,
		},
		{
			name:   "alarm outranks an unreadable value",
			sensor: TemperatureSensor{Celsius: math.NaN(), Alarm: sensorBoolPtr(true)},
			want:   MetricPostureSeverityCritical,
		},
		{
			name:   "critical limit reached",
			sensor: TemperatureSensor{Celsius: 95, MaxCelsius: sensorFloatPtr(80), CriticalCelsius: sensorFloatPtr(95)},
			want:   MetricPostureSeverityCritical,
		},
		{
			name:   "warning limit reached",
			sensor: TemperatureSensor{Celsius: 80, MaxCelsius: sensorFloatPtr(80), CriticalCelsius: sensorFloatPtr(95)},
			want:   MetricPostureSeverityWarning,
		},
		{
			name:   "below every limit",
			sensor: TemperatureSensor{Celsius: 79.9, MaxCelsius: sensorFloatPtr(80), CriticalCelsius: sensorFloatPtr(95)},
			want:   MetricPostureSeverityOK,
		},
		{
			name:   "warning limit only",
			sensor: TemperatureSensor{Celsius: 85, MaxCelsius: sensorFloatPtr(80)},
			want:   MetricPostureSeverityWarning,
		},
		{
			name:   "critical limit only below",
			sensor: TemperatureSensor{Celsius: 70, CriticalCelsius: sensorFloatPtr(95)},
			want:   MetricPostureSeverityOK,
		},
		{
			name:   "inverted limits resolve to critical",
			sensor: TemperatureSensor{Celsius: 85, MaxCelsius: sensorFloatPtr(95), CriticalCelsius: sensorFloatPtr(80)},
			want:   MetricPostureSeverityCritical,
		},
		{
			name:   "inverted limits below both",
			sensor: TemperatureSensor{Celsius: 70, MaxCelsius: sensorFloatPtr(95), CriticalCelsius: sensorFloatPtr(80)},
			want:   MetricPostureSeverityOK,
		},
		{
			name:   "zero limit is no limit",
			sensor: TemperatureSensor{Celsius: 45, MaxCelsius: sensorFloatPtr(0)},
			want:   MetricPostureSeverityUnknown,
		},
		{
			name:   "negative limit is no limit",
			sensor: TemperatureSensor{Celsius: 45, MaxCelsius: sensorFloatPtr(-1), Alarm: sensorBoolPtr(false)},
			want:   MetricPostureSeverityOK,
		},
		{
			name:   "measured without limits or alarm",
			sensor: TemperatureSensor{Celsius: 41},
			want:   MetricPostureSeverityUnknown,
		},
		{
			name:   "cleared alarm without limits",
			sensor: TemperatureSensor{Celsius: 41, Alarm: sensorBoolPtr(false)},
			want:   MetricPostureSeverityOK,
		},
		{
			name:   "sub-zero reading is classified",
			sensor: TemperatureSensor{Celsius: -5, MaxCelsius: sensorFloatPtr(80)},
			want:   MetricPostureSeverityOK,
		},
		{
			name:   "unreadable value without alarm",
			sensor: TemperatureSensor{Celsius: math.Inf(1), MaxCelsius: sensorFloatPtr(80)},
			want:   MetricPostureSeverityUnknown,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := TemperatureSensorStatus(test.sensor); got != test.want {
				t.Fatalf("TemperatureSensorStatus() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestFanSensorStatusUsesAlarmOnly(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		sensor FanSensor
		want   string
	}{
		{
			name:   "spinning fan without an alarm bit",
			sensor: FanSensor{RPM: 5200},
			want:   MetricPostureSeverityUnknown,
		},
		{
			name:   "tachometer range is not a threshold",
			sensor: FanSensor{RPM: 100, MinRPM: sensorInt64Ptr(600), MaxRPM: sensorInt64Ptr(2000)},
			want:   MetricPostureSeverityUnknown,
		},
		{
			name:   "stopped fan with a cleared alarm",
			sensor: FanSensor{RPM: 0, Alarm: sensorBoolPtr(false)},
			want:   MetricPostureSeverityOK,
		},
		{
			name:   "asserted alarm",
			sensor: FanSensor{RPM: 0, Alarm: sensorBoolPtr(true)},
			want:   MetricPostureSeverityCritical,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := FanSensorStatus(test.sensor); got != test.want {
				t.Fatalf("FanSensorStatus() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPowerSensorStatusIgnoresCapWatts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		sensor PowerSensor
		want   string
	}{
		{
			name:   "configured cap is not a failure limit",
			sensor: PowerSensor{Watts: 40, CapWatts: sensorFloatPtr(30)},
			want:   MetricPostureSeverityUnknown,
		},
		{
			name:   "warning limit reached",
			sensor: PowerSensor{Watts: 55, MaxWatts: sensorFloatPtr(50)},
			want:   MetricPostureSeverityWarning,
		},
		{
			name:   "critical limit reached",
			sensor: PowerSensor{Watts: 70, CriticalWatts: sensorFloatPtr(65)},
			want:   MetricPostureSeverityCritical,
		},
		{
			name:   "below limits",
			sensor: PowerSensor{Watts: 40, MaxWatts: sensorFloatPtr(50), CriticalWatts: sensorFloatPtr(60)},
			want:   MetricPostureSeverityOK,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := PowerSensorStatus(test.sensor); got != test.want {
				t.Fatalf("PowerSensorStatus() = %q, want %q", got, test.want)
			}
		})
	}
}
