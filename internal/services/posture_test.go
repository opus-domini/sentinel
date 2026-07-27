package services

import (
	"sync"
	"testing"
	"time"
)

func TestMetricPostureEvaluatorNormalAndUnavailable(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	evaluator := newMetricPostureEvaluator(func() time.Time { return now })

	normal := unavailableHostMetrics()
	normal.CPUPercent = 40
	got := evaluator.Evaluate(normal)
	if got.State != MetricPostureStateNormal || got.Severity != MetricPostureSeverityOK {
		t.Fatalf("normal posture = %+v", got)
	}
	if got.Signals == nil || len(got.Signals) != 0 {
		t.Fatalf("normal signals = %#v, want non-nil empty", got.Signals)
	}
	if got.ObservedAt != "2026-07-27T12:00:00Z" {
		t.Fatalf("observedAt = %q", got.ObservedAt)
	}

	got = evaluator.Evaluate(unavailableHostMetrics())
	if got.State != MetricPostureStateUnavailable ||
		got.Severity != MetricPostureSeverityUnknown {
		t.Fatalf("unavailable posture = %+v", got)
	}
	if got.Signals == nil || len(got.Signals) != 0 {
		t.Fatalf("unavailable signals = %#v, want non-nil empty", got.Signals)
	}
}

func TestMetricPostureEvaluatorVolatileSignalsUseElapsedTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		metrics func(float64) HostMetrics
	}{
		{
			name: "cpu",
			metrics: func(value float64) HostMetrics {
				metrics := unavailableHostMetrics()
				metrics.CPUPercent = value
				return metrics
			},
		},
		{
			name: "memory",
			metrics: func(value float64) HostMetrics {
				metrics := unavailableHostMetrics()
				metrics.MemTotalBytes = 100
				metrics.MemPercent = value
				return metrics
			},
		},
		{
			name: "swap",
			metrics: func(value float64) HostMetrics {
				metrics := unavailableHostMetrics()
				metrics.SwapTotalBytes = 100
				metrics.SwapPercent = value
				return metrics
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
			now := start
			evaluator := newMetricPostureEvaluator(func() time.Time { return now })
			warning := 80.0
			exit := 74.0
			if tt.name == "swap" {
				warning = 20
				exit = 14
			}

			for range 20 {
				got := evaluator.Evaluate(tt.metrics(warning))
				if got.State != MetricPostureStateNormal {
					t.Fatalf("same-time request accelerated %s: %+v", tt.name, got)
				}
			}

			now = start.Add(9 * time.Second)
			if got := evaluator.Evaluate(tt.metrics(warning)); got.State != MetricPostureStateNormal {
				t.Fatalf("posture before duration = %+v", got)
			}

			now = start.Add(10 * time.Second)
			got := evaluator.Evaluate(tt.metrics(warning))
			signal := requirePostureSignal(t, got, tt.name, MetricPostureSeverityWarning)
			if signal.Since != start.Format(time.RFC3339) {
				t.Fatalf("since = %q, want %q", signal.Since, start.Format(time.RFC3339))
			}

			now = start.Add(11 * time.Second)
			got = evaluator.Evaluate(tt.metrics(exit))
			requirePostureSignal(t, got, tt.name, MetricPostureSeverityWarning)

			now = start.Add(20 * time.Second)
			got = evaluator.Evaluate(tt.metrics(exit))
			requirePostureSignal(t, got, tt.name, MetricPostureSeverityWarning)

			now = start.Add(21 * time.Second)
			got = evaluator.Evaluate(tt.metrics(exit))
			if got.State != MetricPostureStateNormal || len(got.Signals) != 0 {
				t.Fatalf("posture after exit duration = %+v", got)
			}
		})
	}
}

func TestMetricPostureEvaluatorVolatileEscalationUsesDuration(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	now := start
	evaluator := newMetricPostureEvaluator(func() time.Time { return now })
	metrics := unavailableHostMetrics()
	metrics.CPUPercent = 80

	evaluator.Evaluate(metrics)
	now = start.Add(10 * time.Second)
	requirePostureSignal(
		t,
		evaluator.Evaluate(metrics),
		"cpu",
		MetricPostureSeverityWarning,
	)

	metrics.CPUPercent = 90
	evaluator.Evaluate(metrics)
	now = start.Add(19 * time.Second)
	requirePostureSignal(
		t,
		evaluator.Evaluate(metrics),
		"cpu",
		MetricPostureSeverityWarning,
	)

	now = start.Add(20 * time.Second)
	signal := requirePostureSignal(
		t,
		evaluator.Evaluate(metrics),
		"cpu",
		MetricPostureSeverityCritical,
	)
	if signal.Since != start.Add(10*time.Second).Format(time.RFC3339) {
		t.Fatalf("critical since = %q", signal.Since)
	}
}

func TestMetricPostureEvaluatorCapacitySignalsAreImmediateAndHysteretic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		warning   float64
		critical  float64
		heldValue float64
		exitValue float64
		metrics   func(float64) HostMetrics
	}{
		{
			name: "rootDisk", warning: 85, critical: 95, heldValue: 83, exitValue: 82.9,
			metrics: func(value float64) HostMetrics {
				metrics := unavailableHostMetrics()
				metrics.DiskTotalBytes = 100
				metrics.DiskPercent = value
				return metrics
			},
		},
		{
			name: "inodes", warning: 80, critical: 90, heldValue: 78, exitValue: 77.9,
			metrics: func(value float64) HostMetrics {
				metrics := unavailableHostMetrics()
				metrics.DiskInodesTotal = 100
				metrics.DiskInodesPercent = value
				return metrics
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
			evaluator := newMetricPostureEvaluator(func() time.Time { return now })

			requirePostureSignal(
				t,
				evaluator.Evaluate(tt.metrics(tt.warning)),
				tt.name,
				MetricPostureSeverityWarning,
			)
			requirePostureSignal(
				t,
				evaluator.Evaluate(tt.metrics(tt.critical)),
				tt.name,
				MetricPostureSeverityCritical,
			)
			requirePostureSignal(
				t,
				evaluator.Evaluate(tt.metrics(tt.heldValue)),
				tt.name,
				MetricPostureSeverityWarning,
			)
			got := evaluator.Evaluate(tt.metrics(tt.exitValue))
			if got.State != MetricPostureStateNormal || len(got.Signals) != 0 {
				t.Fatalf("capacity posture below exit threshold = %+v", got)
			}
		})
	}
}

func TestMetricPostureEvaluatorPressureSignalsExitAfterTenSeconds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		metrics func(float64) HostMetrics
	}{
		{
			name: "cpuPressure",
			metrics: func(value float64) HostMetrics {
				metrics := unavailableHostMetrics()
				metrics.CPUPressureAvg10 = value
				return metrics
			},
		},
		{
			name: "memoryPressure",
			metrics: func(value float64) HostMetrics {
				metrics := unavailableHostMetrics()
				metrics.MemPressureAvg10 = value
				return metrics
			},
		},
		{
			name: "ioPressure",
			metrics: func(value float64) HostMetrics {
				metrics := unavailableHostMetrics()
				metrics.IOPressureAvg10 = value
				return metrics
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
			now := start
			evaluator := newMetricPostureEvaluator(func() time.Time { return now })

			requirePostureSignal(
				t,
				evaluator.Evaluate(tt.metrics(2)),
				tt.name,
				MetricPostureSeverityWarning,
			)
			requirePostureSignal(
				t,
				evaluator.Evaluate(tt.metrics(10)),
				tt.name,
				MetricPostureSeverityCritical,
			)

			now = start.Add(time.Second)
			requirePostureSignal(
				t,
				evaluator.Evaluate(tt.metrics(1.4)),
				tt.name,
				MetricPostureSeverityCritical,
			)
			now = start.Add(11 * time.Second)
			got := evaluator.Evaluate(tt.metrics(1.4))
			if got.State != MetricPostureStateNormal || len(got.Signals) != 0 {
				t.Fatalf("pressure posture after exit duration = %+v", got)
			}
		})
	}
}

func TestMetricPostureEvaluatorIsThreadSafe(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	evaluator := newMetricPostureEvaluator(func() time.Time { return now })
	metrics := unavailableHostMetrics()
	metrics.DiskTotalBytes = 100
	metrics.DiskPercent = 96

	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := evaluator.Evaluate(metrics)
			if got.State != MetricPostureStatePressure {
				t.Errorf("concurrent posture = %+v", got)
			}
		}()
	}
	wg.Wait()
}

func unavailableHostMetrics() HostMetrics {
	return HostMetrics{
		CPUPercent:        -1,
		MemPercent:        -1,
		DiskPercent:       -1,
		DiskInodesPercent: -1,
		SwapPercent:       -1,
		CPUPressureAvg10:  -1,
		MemPressureAvg10:  -1,
		IOPressureAvg10:   -1,
	}
}

func requirePostureSignal(
	t *testing.T,
	posture MetricPosture,
	name string,
	severity string,
) MetricPostureSignal {
	t.Helper()
	for _, signal := range posture.Signals {
		if signal.Name == name {
			if signal.Severity != severity {
				t.Fatalf("%s severity = %q, want %q; posture=%+v", name, signal.Severity, severity, posture)
			}
			if signal.Since == "" {
				t.Fatalf("%s since is empty; posture=%+v", name, posture)
			}
			return signal
		}
	}
	t.Fatalf("%s signal missing from posture %+v", name, posture)
	return MetricPostureSignal{}
}
