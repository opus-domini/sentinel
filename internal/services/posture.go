package services

import (
	"math"
	"strings"
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
	Subject  string  `json:"subject,omitempty"`
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
	name           string
	subject        string
	value          float64
	signalValue    float64
	hasSignalValue bool
	warning        float64
	critical       float64
	exitMargin     float64
	enterDuration  time.Duration
	exitDuration   time.Duration
	available      bool
}

type metricSignalState struct {
	subject       string
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
	policies := metricSignalPolicies(metrics)
	seen := make(map[string]struct{}, len(policies))
	for _, policy := range policies {
		seen[policy.name] = struct{}{}
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
	for name := range e.states {
		if _, exists := seen[name]; !exists {
			delete(e.states, name)
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
	if state == nil || state.subject != policy.subject {
		state = &metricSignalState{subject: policy.subject}
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

	value := policy.value
	if policy.hasSignalValue {
		value = policy.signalValue
	}
	return MetricPostureSignal{
		Name:     policy.name,
		Subject:  policy.subject,
		Severity: state.severity,
		Value:    value,
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
	policies := []metricSignalPolicy{
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
	if policy, ok := temperatureSignalPolicy(metrics.Sensors.Temperatures); ok {
		policies = append(policies, policy)
	}
	if policy, ok := fanSignalPolicy(metrics.Sensors.Fans); ok {
		policies = append(policies, policy)
	}
	if policy, ok := powerSignalPolicy(metrics.Sensors.Power); ok {
		policies = append(policies, policy)
	}
	return policies
}

type sensorPolicyCandidate struct {
	policy    metricSignalPolicy
	severity  int
	proximity float64
}

func temperatureSignalPolicy(sensors []TemperatureSensor) (metricSignalPolicy, bool) {
	candidates := make([]sensorPolicyCandidate, 0, len(sensors))
	for _, sensor := range sensors {
		if !validMetricValue(sensor.Celsius) {
			continue
		}
		policy, ok := thresholdSensorPolicy(
			"temperature",
			sensorSubject(sensor.Label, sensor.Source),
			sensor.Celsius,
			sensor.MaxCelsius,
			sensor.CriticalCelsius,
			sensor.Alarm,
			3,
		)
		if ok {
			candidates = append(candidates, newSensorPolicyCandidate(policy))
		}
	}
	return selectSensorPolicy(candidates)
}

func fanSignalPolicy(sensors []FanSensor) (metricSignalPolicy, bool) {
	candidates := make([]sensorPolicyCandidate, 0, len(sensors))
	for _, sensor := range sensors {
		if sensor.RPM < 0 || sensor.Alarm == nil {
			continue
		}
		policy := alarmSensorPolicy(
			"fan",
			sensorSubject(sensor.Label, sensor.Source),
			float64(sensor.RPM),
			*sensor.Alarm,
		)
		candidates = append(candidates, newSensorPolicyCandidate(policy))
	}
	return selectSensorPolicy(candidates)
}

func powerSignalPolicy(sensors []PowerSensor) (metricSignalPolicy, bool) {
	candidates := make([]sensorPolicyCandidate, 0, len(sensors))
	for _, sensor := range sensors {
		if !validMetricValue(sensor.Watts) {
			continue
		}
		policy, ok := thresholdSensorPolicy(
			"power",
			sensorSubject(sensor.Label, sensor.Source),
			sensor.Watts,
			sensor.MaxWatts,
			sensor.CriticalWatts,
			sensor.Alarm,
			0,
		)
		if !ok {
			continue
		}
		if policy.exitMargin == 0 && policy.warning > 0.5 {
			policy.exitMargin = policy.warning * 0.05
		}
		candidates = append(candidates, newSensorPolicyCandidate(policy))
	}
	return selectSensorPolicy(candidates)
}

func thresholdSensorPolicy(
	name string,
	subject string,
	value float64,
	warningValue *float64,
	criticalValue *float64,
	alarm *bool,
	exitMargin float64,
) (metricSignalPolicy, bool) {
	if alarm != nil && *alarm {
		return alarmSensorPolicy(name, subject, value, true), true
	}

	warning, hasWarning := validThreshold(warningValue)
	critical, hasCritical := validThreshold(criticalValue)
	if hasWarning && hasCritical && warning > critical {
		warning = critical
	}
	switch {
	case hasWarning && !hasCritical:
		critical = math.MaxFloat64
	case !hasWarning && hasCritical:
		warning = critical
	case !hasWarning && !hasCritical:
		if alarm == nil {
			return metricSignalPolicy{}, false
		}
		return alarmSensorPolicy(name, subject, value, false), true
	}
	return metricSignalPolicy{
		name:           name,
		subject:        subject,
		value:          value,
		signalValue:    value,
		hasSignalValue: true,
		warning:        warning,
		critical:       critical,
		exitMargin:     exitMargin,
		exitDuration:   pressureExitDuration,
		available:      true,
	}, true
}

func alarmSensorPolicy(name, subject string, signalValue float64, alarm bool) metricSignalPolicy {
	value := 0.0
	if alarm {
		value = 1
	}
	return metricSignalPolicy{
		name:           name,
		subject:        subject,
		value:          value,
		signalValue:    signalValue,
		hasSignalValue: true,
		warning:        0.5,
		critical:       0.5,
		exitDuration:   pressureExitDuration,
		available:      true,
	}
}

func validThreshold(value *float64) (float64, bool) {
	if value == nil || !validMetricValue(*value) || *value <= 0 {
		return 0, false
	}
	return *value, true
}

func newSensorPolicyCandidate(policy metricSignalPolicy) sensorPolicyCandidate {
	severity := 1
	switch {
	case policy.value >= policy.critical:
		severity = 3
	case policy.value >= policy.warning:
		severity = 2
	}
	proximity := 0.0
	if policy.warning > 0 {
		proximity = policy.value / policy.warning
	}
	return sensorPolicyCandidate{policy: policy, severity: severity, proximity: proximity}
}

func selectSensorPolicy(candidates []sensorPolicyCandidate) (metricSignalPolicy, bool) {
	if len(candidates) == 0 {
		return metricSignalPolicy{}, false
	}
	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.severity > best.severity ||
			(candidate.severity == best.severity && candidate.proximity > best.proximity) ||
			(candidate.severity == best.severity && candidate.proximity == best.proximity &&
				candidate.policy.subject < best.policy.subject) {
			best = candidate
		}
	}
	return best.policy, true
}

func sensorSubject(label, source string) string {
	label = strings.TrimSpace(label)
	source = strings.TrimSpace(source)
	if label == "" {
		return source
	}
	if source == "" || strings.EqualFold(label, source) {
		return label
	}
	return label + " (" + source + ")"
}

func validMetricValue(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
