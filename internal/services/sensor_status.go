package services

import "math"

// TemperatureSensorStatus classifies one temperature reading against the limits
// its hardware published. Unlike the metric posture it is stateless and applies
// no entry or exit hysteresis, so the status always agrees with the reading
// reported beside it.
func TemperatureSensorStatus(sensor TemperatureSensor) string {
	return sensorStatus(sensor.Celsius, sensor.MaxCelsius, sensor.CriticalCelsius, sensor.Alarm)
}

// FanSensorStatus classifies one fan reading. MinRPM and MaxRPM describe the
// tachometer range rather than failure limits, so a fan without a hardware
// alarm bit stays unclassifiable and zero RPM is never an invented failure.
func FanSensorStatus(sensor FanSensor) string {
	return sensorStatus(float64(sensor.RPM), nil, nil, sensor.Alarm)
}

// PowerSensorStatus classifies one power reading. CapWatts is a configured
// budget rather than a failure limit and is deliberately not used.
func PowerSensorStatus(sensor PowerSensor) string {
	return sensorStatus(sensor.Watts, sensor.MaxWatts, sensor.CriticalWatts, sensor.Alarm)
}

// sensorStatus judges a single reading. An asserted hardware alarm outranks
// every numeric limit, matching thresholdSensorPolicy. Limits are validated
// with validThreshold so posture and this status cannot disagree about which
// limits are usable, and critical is tested before warning, which reaches the
// same verdict as the clamp thresholdSensorPolicy applies to an inverted pair.
//
// Two deliberate differences from the metrics UI in
// frontend/src/lib/metricsView.ts: an asserted alarm outranks a non-finite
// reading here, and a limit of zero or less is no limit at all rather than a
// limit every reading meets.
func sensorStatus(value float64, warningLimit, criticalLimit *float64, alarm *bool) string {
	if alarm != nil && *alarm {
		return MetricPostureSeverityCritical
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return MetricPostureSeverityUnknown
	}
	warning, hasWarning := validThreshold(warningLimit)
	critical, hasCritical := validThreshold(criticalLimit)
	switch {
	case hasCritical && value >= critical:
		return MetricPostureSeverityCritical
	case hasWarning && value >= warning:
		return MetricPostureSeverityWarning
	case hasWarning || hasCritical || alarm != nil:
		return MetricPostureSeverityOK
	default:
		return MetricPostureSeverityUnknown
	}
}
