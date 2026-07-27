package services

import (
	"math"
	"sync"
	"time"
)

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

	volatileSignalDuration = 10 * time.Second
	pressureExitDuration   = 10 * time.Second
)

// MetricPostureSignal identifies one host signal currently under pressure.
type MetricPostureSignal struct {
	Name     string  `json:"name"`
	Severity string  `json:"severity"`
	Value    float64 `json:"value"`
	Since    string  `json:"since"`
}

// MetricPosture is the canonical aggregate health assessment for host metrics.
type MetricPosture struct {
	State         string                `json:"state"`
	Severity      string                `json:"severity"`
	WarningCount  int                   `json:"warningCount"`
	CriticalCount int                   `json:"criticalCount"`
	Signals       []MetricPostureSignal `json:"signals"`
	ObservedAt    string                `json:"observedAt"`
}

// MetricsSnapshot keeps the raw sample and its stateful posture together.
type MetricsSnapshot struct {
	Metrics HostMetrics   `json:"metrics"`
	Posture MetricPosture `json:"posture"`
}

type metricSignalPolicy struct {
	name          string
	value         float64
	warning       float64
	critical      float64
	exitMargin    float64
	enterDuration time.Duration
	exitDuration  time.Duration
	available     bool
}

type metricSignalState struct {
	severity      string
	since         time.Time
	aboveWarning  time.Time
	aboveCritical time.Time
	belowExit     time.Time
}

type metricPostureEvaluator struct {
	mu     sync.Mutex
	nowFn  func() time.Time
	states map[string]*metricSignalState
}

func newMetricPostureEvaluator(nowFn func() time.Time) *metricPostureEvaluator {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &metricPostureEvaluator{
		nowFn:  nowFn,
		states: make(map[string]*metricSignalState),
	}
}

func (e *metricPostureEvaluator) Evaluate(metrics HostMetrics) MetricPosture {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := e.nowFn().UTC()
	posture := MetricPosture{
		State:      MetricPostureStateNormal,
		Severity:   MetricPostureSeverityOK,
		Signals:    make([]MetricPostureSignal, 0),
		ObservedAt: now.Format(time.RFC3339),
	}

	evaluated := 0
	for _, policy := range metricSignalPolicies(metrics) {
		if policy.available {
			evaluated++
		}
		signal, active := e.evaluateSignal(policy, now)
		if !active {
			continue
		}
		posture.Signals = append(posture.Signals, signal)
		switch signal.Severity {
		case MetricPostureSeverityCritical:
			posture.CriticalCount++
		case MetricPostureSeverityWarning:
			posture.WarningCount++
		}
	}

	switch {
	case evaluated == 0:
		posture.State = MetricPostureStateUnavailable
		posture.Severity = MetricPostureSeverityUnknown
	case posture.CriticalCount > 0:
		posture.State = MetricPostureStatePressure
		posture.Severity = MetricPostureSeverityCritical
	case posture.WarningCount > 0:
		posture.State = MetricPostureStatePressure
		posture.Severity = MetricPostureSeverityWarning
	}
	return posture
}

func (e *metricPostureEvaluator) evaluateSignal(
	policy metricSignalPolicy,
	now time.Time,
) (MetricPostureSignal, bool) {
	if !policy.available {
		delete(e.states, policy.name)
		return MetricPostureSignal{}, false
	}

	state := e.states[policy.name]
	if state == nil {
		state = &metricSignalState{}
		e.states[policy.name] = state
	}
	updateThresholdSince(&state.aboveWarning, policy.value >= policy.warning, now)
	updateThresholdSince(&state.aboveCritical, policy.value >= policy.critical, now)

	if state.severity == "" {
		severity, since := eligibleSeverity(policy, state, now)
		if severity == "" {
			return MetricPostureSignal{}, false
		}
		state.severity = severity
		state.since = since
	}

	exitThreshold := policy.warning - policy.exitMargin
	if policy.value < exitThreshold {
		if state.belowExit.IsZero() {
			state.belowExit = now
		}
		if now.Sub(state.belowExit) >= policy.exitDuration {
			delete(e.states, policy.name)
			return MetricPostureSignal{}, false
		}
	} else {
		state.belowExit = time.Time{}
		switch {
		case state.severity == MetricPostureSeverityWarning &&
			thresholdElapsed(state.aboveCritical, policy.enterDuration, now):
			state.severity = MetricPostureSeverityCritical
			state.since = state.aboveCritical
		case state.severity == MetricPostureSeverityCritical &&
			policy.value < policy.critical:
			state.severity = MetricPostureSeverityWarning
			state.since = now
		}
	}

	return MetricPostureSignal{
		Name:     policy.name,
		Severity: state.severity,
		Value:    policy.value,
		Since:    state.since.UTC().Format(time.RFC3339),
	}, true
}

func eligibleSeverity(
	policy metricSignalPolicy,
	state *metricSignalState,
	now time.Time,
) (string, time.Time) {
	if thresholdElapsed(state.aboveCritical, policy.enterDuration, now) {
		return MetricPostureSeverityCritical, state.aboveCritical
	}
	if thresholdElapsed(state.aboveWarning, policy.enterDuration, now) {
		return MetricPostureSeverityWarning, state.aboveWarning
	}
	return "", time.Time{}
}

func thresholdElapsed(since time.Time, duration time.Duration, now time.Time) bool {
	return !since.IsZero() && now.Sub(since) >= duration
}

func updateThresholdSince(since *time.Time, above bool, now time.Time) {
	if !above {
		*since = time.Time{}
		return
	}
	if since.IsZero() {
		*since = now
	}
}

func metricSignalPolicies(metrics HostMetrics) []metricSignalPolicy {
	return []metricSignalPolicy{
		{
			name: "cpu", value: metrics.CPUPercent, warning: 80, critical: 90,
			exitMargin: 5, enterDuration: volatileSignalDuration,
			exitDuration: volatileSignalDuration,
			available:    validMetricValue(metrics.CPUPercent),
		},
		{
			name: "memory", value: metrics.MemPercent, warning: 80, critical: 90,
			exitMargin: 5, enterDuration: volatileSignalDuration,
			exitDuration: volatileSignalDuration,
			available: metrics.MemTotalBytes > 0 &&
				validMetricValue(metrics.MemPercent),
		},
		{
			name: "rootDisk", value: metrics.DiskPercent, warning: 85, critical: 95,
			exitMargin: 2,
			available: metrics.DiskTotalBytes > 0 &&
				validMetricValue(metrics.DiskPercent),
		},
		{
			name: "inodes", value: metrics.DiskInodesPercent, warning: 80, critical: 90,
			exitMargin: 2,
			available: metrics.DiskInodesTotal > 0 &&
				validMetricValue(metrics.DiskInodesPercent),
		},
		{
			name: "swap", value: metrics.SwapPercent, warning: 20, critical: 60,
			exitMargin: 5, enterDuration: volatileSignalDuration,
			exitDuration: volatileSignalDuration,
			available: metrics.SwapTotalBytes > 0 &&
				validMetricValue(metrics.SwapPercent),
		},
		{
			name: "cpuPressure", value: metrics.CPUPressureAvg10, warning: 2, critical: 10,
			exitMargin: 0.5, exitDuration: pressureExitDuration,
			available: validMetricValue(metrics.CPUPressureAvg10),
		},
		{
			name: "memoryPressure", value: metrics.MemPressureAvg10, warning: 2, critical: 10,
			exitMargin: 0.5, exitDuration: pressureExitDuration,
			available: validMetricValue(metrics.MemPressureAvg10),
		},
		{
			name: "ioPressure", value: metrics.IOPressureAvg10, warning: 2, critical: 10,
			exitMargin: 0.5, exitDuration: pressureExitDuration,
			available: validMetricValue(metrics.IOPressureAvg10),
		},
	}
}

func validMetricValue(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
