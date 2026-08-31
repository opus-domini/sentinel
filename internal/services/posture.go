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

// stateKey identifies the temporal state this policy accumulates. Sensor
// categories evaluate every physical sensor separately, so the subject is part
// of the identity; single-source signals carry no subject.
func (p metricSignalPolicy) stateKey() string {
	if p.subject == "" {
		return p.name
	}
	return p.name + "\x00" + p.subject
}

// proximity is how close the reading sits to its own warning threshold, which
// is the only scale-free way to rank sensors with different limits.
func (p metricSignalPolicy) proximity() float64 {
	if p.warning <= 0 {
		return 0
	}
	return p.value / p.warning
}

type metricSignalState struct {
	severity      string
	since         time.Time
	aboveWarning  time.Time
	aboveCritical time.Time
	belowExit     time.Time
}

// evaluatedSignal pairs an active signal with the policy that produced it, so
// the per-category projection can rank sensors against their own thresholds.
type evaluatedSignal struct {
	policy metricSignalPolicy
	signal MetricPostureSignal
}

// worseThan reports whether s outranks other as the reported signal for their
// shared name: higher severity first, then the reading closest to its own
// warning threshold, then a stable subject order.
func (s evaluatedSignal) worseThan(other evaluatedSignal) bool {
	rank, otherRank := severityRank(s.signal.Severity), severityRank(other.signal.Severity)
	if rank != otherRank {
		return rank > otherRank
	}
	if proximity, otherProximity := s.policy.proximity(), other.policy.proximity(); proximity != otherProximity {
		return proximity > otherProximity
	}
	return s.policy.subject < other.policy.subject
}

func severityRank(severity string) int {
	switch severity {
	case MetricPostureSeverityCritical:
		return 2
	case MetricPostureSeverityWarning:
		return 1
	default:
		return 0
	}
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
	active := make([]evaluatedSignal, 0, len(policies))
	for _, policy := range policies {
		seen[policy.stateKey()] = struct{}{}
		if policy.available {
			evaluated++
		}
		signal, ok := e.evaluateSignal(policy, now)
		if !ok {
			continue
		}
		active = append(active, evaluatedSignal{policy: policy, signal: signal})
	}
	for key := range e.states {
		if _, exists := seen[key]; !exists {
			delete(e.states, key)
		}
	}

	for _, signal := range projectWorstPerName(active) {
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

// projectWorstPerName keeps one signal per policy name. Sensor categories track
// hysteresis per physical sensor — the hottest core changes constantly, and
// resetting the dwell timers on every swap would defeat it — but the posture
// still reports a single signal per category: the worst one currently active.
func projectWorstPerName(active []evaluatedSignal) []MetricPostureSignal {
	order := make([]string, 0, len(active))
	worst := make(map[string]evaluatedSignal, len(active))
	for _, candidate := range active {
		current, exists := worst[candidate.signal.Name]
		if !exists {
			order = append(order, candidate.signal.Name)
			worst[candidate.signal.Name] = candidate
			continue
		}
		if candidate.worseThan(current) {
			worst[candidate.signal.Name] = candidate
		}
	}

	signals := make([]MetricPostureSignal, 0, len(order))
	for _, name := range order {
		signals = append(signals, worst[name].signal)
	}
	return signals
}

func (e *metricPostureEvaluator) evaluateSignal(
	policy metricSignalPolicy,
	now time.Time,
) (MetricPostureSignal, bool) {
	key := policy.stateKey()
	if !policy.available {
		delete(e.states, key)
		return MetricPostureSignal{}, false
	}

	state := e.states[key]
	if state == nil {
		state = &metricSignalState{}
		e.states[key] = state
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
			delete(e.states, key)
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
	policies = append(policies, temperatureSignalPolicies(metrics.Sensors.Temperatures)...)
	policies = append(policies, fanSignalPolicies(metrics.Sensors.Fans)...)
	policies = append(policies, powerSignalPolicies(metrics.Sensors.Power)...)
	return policies
}

func temperatureSignalPolicies(sensors []TemperatureSensor) []metricSignalPolicy {
	policies := make([]metricSignalPolicy, 0, len(sensors))
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
			policies = append(policies, policy)
		}
	}
	return policies
}

func fanSignalPolicies(sensors []FanSensor) []metricSignalPolicy {
	policies := make([]metricSignalPolicy, 0, len(sensors))
	for _, sensor := range sensors {
		if sensor.RPM < 0 || sensor.Alarm == nil {
			continue
		}
		policies = append(policies, alarmSensorPolicy(
			"fan",
			sensorSubject(sensor.Label, sensor.Source),
			float64(sensor.RPM),
			*sensor.Alarm,
		))
	}
	return policies
}

func powerSignalPolicies(sensors []PowerSensor) []metricSignalPolicy {
	policies := make([]metricSignalPolicy, 0, len(sensors))
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
		policies = append(policies, policy)
	}
	return policies
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
