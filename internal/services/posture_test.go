package services

import (
	"math"
	"reflect"
	"testing"
)

func TestEvaluateMetricPosture(t *testing.T) {
	t.Parallel()

	normal := HostMetrics{
		CPUPercent:        40,
		MemTotalBytes:     100,
		MemPercent:        50,
		DiskTotalBytes:    100,
		DiskPercent:       60,
		DiskInodesTotal:   100,
		DiskInodesPercent: 70,
		SwapTotalBytes:    100,
		SwapPercent:       10,
		CPUPressureAvg10:  1,
		MemPressureAvg10:  1,
		IOPressureAvg10:   1,
	}

	tests := []struct {
		name    string
		metrics HostMetrics
		want    MetricPosture
	}{
		{
			name:    "normal",
			metrics: normal,
			want: MetricPosture{
				State: MetricPostureStateNormal, Severity: MetricPostureSeverityOK,
				Signals: []MetricPostureSignal{},
			},
		},
		{
			name: "warning",
			metrics: func() HostMetrics {
				sample := normal
				sample.CPUPercent = 80
				return sample
			}(),
			want: MetricPosture{
				State: MetricPostureStatePressure, Severity: MetricPostureSeverityWarning,
				WarningCount: 1,
				Signals: []MetricPostureSignal{{
					Name: "cpu", Severity: MetricPostureSeverityWarning, Value: 80,
				}},
			},
		},
		{
			name: "warning and critical",
			metrics: func() HostMetrics {
				sample := normal
				sample.CPUPercent = 85
				sample.MemPercent = 95
				return sample
			}(),
			want: MetricPosture{
				State: MetricPostureStatePressure, Severity: MetricPostureSeverityCritical,
				WarningCount: 1, CriticalCount: 1,
				Signals: []MetricPostureSignal{
					{Name: "cpu", Severity: MetricPostureSeverityWarning, Value: 85},
					{Name: "memory", Severity: MetricPostureSeverityCritical, Value: 95},
				},
			},
		},
		{
			name: "partial",
			metrics: HostMetrics{
				CPUPercent:        -1,
				MemTotalBytes:     100,
				MemPercent:        40,
				CPUPressureAvg10:  -1,
				MemPressureAvg10:  -1,
				IOPressureAvg10:   -1,
				DiskPercent:       math.NaN(),
				DiskInodesPercent: math.Inf(1),
			},
			want: MetricPosture{
				State: MetricPostureStateNormal, Severity: MetricPostureSeverityOK,
				Signals: []MetricPostureSignal{},
			},
		},
		{
			name: "swap absent",
			metrics: HostMetrics{
				CPUPercent:        20,
				SwapPercent:       99,
				CPUPressureAvg10:  -1,
				MemPressureAvg10:  -1,
				IOPressureAvg10:   -1,
				DiskInodesPercent: -1,
			},
			want: MetricPosture{
				State: MetricPostureStateNormal, Severity: MetricPostureSeverityOK,
				Signals: []MetricPostureSignal{},
			},
		},
		{
			name: "pressure information absent",
			metrics: HostMetrics{
				CPUPercent:       20,
				CPUPressureAvg10: -1,
				MemPressureAvg10: -1,
				IOPressureAvg10:  -1,
			},
			want: MetricPosture{
				State: MetricPostureStateNormal, Severity: MetricPostureSeverityOK,
				Signals: []MetricPostureSignal{},
			},
		},
		{
			name: "unavailable",
			metrics: HostMetrics{
				CPUPercent:        -1,
				MemPercent:        -1,
				DiskPercent:       -1,
				DiskInodesPercent: -1,
				SwapPercent:       -1,
				CPUPressureAvg10:  -1,
				MemPressureAvg10:  -1,
				IOPressureAvg10:   -1,
			},
			want: MetricPosture{
				State: MetricPostureStateUnavailable, Severity: MetricPostureSeverityUnknown,
				Signals: []MetricPostureSignal{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := EvaluateMetricPosture(tt.metrics); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("EvaluateMetricPosture() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
