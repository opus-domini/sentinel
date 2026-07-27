package services

import "math"

const (
	// MetricPostureStateNormal identifies an evaluable sample without pressure.
	MetricPostureStateNormal = "normal"
	// MetricPostureStatePressure identifies an evaluable sample with pressure.
	MetricPostureStatePressure = "pressure"
	// MetricPostureStateUnavailable identifies a sample without evaluable signals.
	MetricPostureStateUnavailable = "unavailable"

	// MetricPostureSeverityOK identifies normal host posture.
	MetricPostureSeverityOK = "ok"
	// MetricPostureSeverityWarning identifies warning host pressure.
	MetricPostureSeverityWarning = "warning"
	// MetricPostureSeverityCritical identifies critical host pressure.
	MetricPostureSeverityCritical = "critical"
	// MetricPostureSeverityUnknown identifies unavailable host posture.
	MetricPostureSeverityUnknown = "unknown"
)

// MetricPostureSignal identifies one host signal currently under pressure.
type MetricPostureSignal struct {
	Name     string  `json:"name"`
	Severity string  `json:"severity"`
	Value    float64 `json:"value"`
}

// MetricPosture is the canonical aggregate health assessment for host metrics.
type MetricPosture struct {
	State         string                `json:"state"`
	Severity      string                `json:"severity"`
	WarningCount  int                   `json:"warningCount"`
	CriticalCount int                   `json:"criticalCount"`
	Signals       []MetricPostureSignal `json:"signals"`
}

type postureCandidate struct {
	name      string
	value     float64
	warning   float64
	critical  float64
	available bool
}

// EvaluateMetricPosture evaluates the host signals shared by Metrics and Now.
func EvaluateMetricPosture(metrics HostMetrics) MetricPosture {
	candidates := []postureCandidate{
		{name: "cpu", value: metrics.CPUPercent, warning: 80, critical: 90, available: validMetricValue(metrics.CPUPercent)},
		{name: "memory", value: metrics.MemPercent, warning: 80, critical: 90, available: metrics.MemTotalBytes > 0 && validMetricValue(metrics.MemPercent)},
		{name: "rootDisk", value: metrics.DiskPercent, warning: 85, critical: 95, available: metrics.DiskTotalBytes > 0 && validMetricValue(metrics.DiskPercent)},
		{name: "inodes", value: metrics.DiskInodesPercent, warning: 80, critical: 90, available: metrics.DiskInodesTotal > 0 && validMetricValue(metrics.DiskInodesPercent)},
		{name: "swap", value: metrics.SwapPercent, warning: 20, critical: 60, available: metrics.SwapTotalBytes > 0 && validMetricValue(metrics.SwapPercent)},
		{name: "cpuPressure", value: metrics.CPUPressureAvg10, warning: 2, critical: 10, available: validMetricValue(metrics.CPUPressureAvg10)},
		{name: "memoryPressure", value: metrics.MemPressureAvg10, warning: 2, critical: 10, available: validMetricValue(metrics.MemPressureAvg10)},
		{name: "ioPressure", value: metrics.IOPressureAvg10, warning: 2, critical: 10, available: validMetricValue(metrics.IOPressureAvg10)},
	}

	posture := MetricPosture{
		State:    MetricPostureStateNormal,
		Severity: MetricPostureSeverityOK,
		Signals:  make([]MetricPostureSignal, 0),
	}
	evaluated := 0
	for _, candidate := range candidates {
		if !candidate.available {
			continue
		}
		evaluated++
		severity := ""
		switch {
		case candidate.value >= candidate.critical:
			severity = MetricPostureSeverityCritical
			posture.CriticalCount++
		case candidate.value >= candidate.warning:
			severity = MetricPostureSeverityWarning
			posture.WarningCount++
		}
		if severity != "" {
			posture.Signals = append(posture.Signals, MetricPostureSignal{
				Name:     candidate.name,
				Severity: severity,
				Value:    candidate.value,
			})
		}
	}

	if evaluated == 0 {
		posture.State = MetricPostureStateUnavailable
		posture.Severity = MetricPostureSeverityUnknown
		return posture
	}
	if posture.CriticalCount > 0 {
		posture.State = MetricPostureStatePressure
		posture.Severity = MetricPostureSeverityCritical
		return posture
	}
	if posture.WarningCount > 0 {
		posture.State = MetricPostureStatePressure
		posture.Severity = MetricPostureSeverityWarning
	}
	return posture
}

func validMetricValue(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
