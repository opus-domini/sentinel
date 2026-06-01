package services

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestCollectMetricsCollectedAt(t *testing.T) {
	t.Parallel()

	m := CollectMetrics(context.Background(), "/")
	if m.CollectedAt == "" {
		t.Fatal("CollectedAt is empty")
	}
	if _, err := time.Parse(time.RFC3339, m.CollectedAt); err != nil {
		t.Fatalf("CollectedAt is not valid RFC3339: %v", err)
	}
}

func TestCollectMetricsGoroutines(t *testing.T) {
	t.Parallel()

	m := CollectMetrics(context.Background(), "/")
	if m.NumGoroutines <= 0 {
		t.Fatalf("NumGoroutines = %d, want > 0", m.NumGoroutines)
	}
}

func TestCollectMetricsGoMemAlloc(t *testing.T) {
	t.Parallel()

	m := CollectMetrics(context.Background(), "/")
	if m.GoMemAllocMB <= 0 {
		t.Fatalf("GoMemAllocMB = %f, want > 0", m.GoMemAllocMB)
	}
}

func TestCollectMetricsMemTotal(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("memory info only tested on linux and darwin")
	}

	m := CollectMetrics(context.Background(), "/")
	if m.MemTotalBytes <= 0 {
		t.Fatalf("MemTotalBytes = %d, want > 0", m.MemTotalBytes)
	}
}

func TestCollectMetricsCPURange(t *testing.T) {
	t.Parallel()

	m := CollectMetrics(context.Background(), "/")
	// -1 is the sentinel for unsupported platforms.
	if m.CPUPercent != -1 && (m.CPUPercent < 0 || m.CPUPercent > 100) {
		t.Fatalf("CPUPercent = %f, want in [0,100] or -1", m.CPUPercent)
	}
}

func TestCollectMetricsExtendedHostSignals(t *testing.T) {
	t.Parallel()

	m := CollectMetrics(context.Background(), "/")
	if m.CPUCount != runtime.NumCPU() {
		t.Fatalf("CPUCount = %d, want %d", m.CPUCount, runtime.NumCPU())
	}
	if m.LoadAvg1 >= 0 && m.LoadPerCPU < 0 {
		t.Fatalf("LoadPerCPU = %f, want non-negative when load is available", m.LoadPerCPU)
	}
	if m.DiskTotalBytes > 0 && m.DiskFreeBytes < 0 {
		t.Fatalf("DiskFreeBytes = %d, want non-negative", m.DiskFreeBytes)
	}
	if m.SwapTotalBytes > 0 && (m.SwapPercent < 0 || m.SwapPercent > 100) {
		t.Fatalf("SwapPercent = %f, want in [0,100]", m.SwapPercent)
	}
	if m.GoMemSysMB <= 0 {
		t.Fatalf("GoMemSysMB = %f, want > 0", m.GoMemSysMB)
	}
}

func TestCollectMetricsLinuxHostSignals(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "linux" {
		t.Skip("linux /proc metrics only tested on linux")
	}

	m := CollectMetrics(context.Background(), "/")
	if m.ProcessCount <= 0 {
		t.Fatalf("ProcessCount = %d, want > 0", m.ProcessCount)
	}
	if m.ThreadCount < m.ProcessCount {
		t.Fatalf("ThreadCount = %d, want >= process count %d", m.ThreadCount, m.ProcessCount)
	}
	if m.HostUptimeSec <= 0 {
		t.Fatalf("HostUptimeSec = %d, want > 0", m.HostUptimeSec)
	}
	if m.CPUPressureAvg10 < 0 && m.CPUPressureAvg10 != -1 {
		t.Fatalf("CPUPressureAvg10 = %f, want >= 0 or -1", m.CPUPressureAvg10)
	}
}

func TestMetricsCollectorReusesRecentSnapshot(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	calls := 0
	collector := newMetricsCollectorWith(
		func() time.Time { return now },
		metricsCollectionIntervals{snapshotReuse: time.Second},
		fakeMetricCollectors(func(context.Context) processSample {
			return processSample{processes: 10, threads: 20, complete: true}
		}, func() float64 {
			calls++
			return float64(calls)
		}),
	)

	first := collector.Collect(context.Background(), "/")
	now = now.Add(500 * time.Millisecond)
	second := collector.Collect(context.Background(), "/")

	if calls != 1 {
		t.Fatalf("cpu collector calls = %d, want 1", calls)
	}
	if second.CPUPercent != first.CPUPercent {
		t.Fatalf("CPUPercent = %f, want cached %f", second.CPUPercent, first.CPUPercent)
	}
	if second.CollectedAt != first.CollectedAt {
		t.Fatalf("CollectedAt = %s, want cached %s", second.CollectedAt, first.CollectedAt)
	}
}

func TestMetricsCollectorUsesSlowerProcessInterval(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	cpuCalls := 0
	processCalls := 0
	collector := newMetricsCollectorWith(
		func() time.Time { return now },
		metricsCollectionIntervals{
			snapshotReuse: time.Nanosecond,
			process:       10 * time.Second,
		},
		fakeMetricCollectors(func(context.Context) processSample {
			processCalls++
			return processSample{processes: processCalls, threads: processCalls * 2, complete: true}
		}, func() float64 {
			cpuCalls++
			return float64(cpuCalls)
		}),
	)

	first := collector.Collect(context.Background(), "/")
	now = now.Add(2 * time.Second)
	second := collector.Collect(context.Background(), "/")
	now = now.Add(9 * time.Second)
	third := collector.Collect(context.Background(), "/")

	if cpuCalls != 3 {
		t.Fatalf("cpu collector calls = %d, want 3", cpuCalls)
	}
	if processCalls != 2 {
		t.Fatalf("process collector calls = %d, want 2", processCalls)
	}
	if first.ProcessCount != 1 || second.ProcessCount != 1 || third.ProcessCount != 2 {
		t.Fatalf("process counts = %d, %d, %d; want 1, 1, 2", first.ProcessCount, second.ProcessCount, third.ProcessCount)
	}
	if first.CPUPercent != 1 || second.CPUPercent != 2 || third.CPUPercent != 3 {
		t.Fatalf("cpu percents = %f, %f, %f; want 1, 2, 3", first.CPUPercent, second.CPUPercent, third.CPUPercent)
	}
}

func TestMetricsCollectorKeepsPreviousProcessSampleWhenRefreshIsIncomplete(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	processCalls := 0
	collector := newMetricsCollectorWith(
		func() time.Time { return now },
		metricsCollectionIntervals{
			snapshotReuse: time.Nanosecond,
			process:       time.Nanosecond,
		},
		fakeMetricCollectors(func(context.Context) processSample {
			processCalls++
			if processCalls == 1 {
				return processSample{processes: 10, threads: 20, complete: true}
			}
			return processSample{processes: 99, threads: 99, complete: false}
		}, func() float64 { return 1 }),
	)

	first := collector.Collect(context.Background(), "/")
	now = now.Add(time.Second)
	second := collector.Collect(context.Background(), "/")

	if first.ProcessCount != 10 {
		t.Fatalf("first ProcessCount = %d, want 10", first.ProcessCount)
	}
	if second.ProcessCount != 10 {
		t.Fatalf("second ProcessCount = %d, want previous complete sample 10", second.ProcessCount)
	}
}

func fakeMetricCollectors(processInfo func(context.Context) processSample, cpuPercent func() float64) metricCollectors {
	return metricCollectors{
		cpuPercent: func(context.Context) float64 {
			return cpuPercent()
		},
		memInfo: func(context.Context) memorySample {
			return memorySample{usedBytes: 25, totalBytes: 100, availableBytes: 75}
		},
		loadAvg: func(context.Context) (float64, float64, float64) {
			return 1, 2, 3
		},
		diskUsage: func(string) diskSample {
			return diskSample{usedBytes: 50, totalBytes: 100, freeBytes: 50, inodesUsed: 5, inodesTotal: 10}
		},
		networkIO: func() networkIOSample {
			return networkIOSample{rxBytes: 100, txBytes: 200, interfaces: 1}
		},
		processInfo: processInfo,
		hostUptime: func() uptimeSample {
			return uptimeSample{uptimeSec: 100, bootTime: "2026-05-13T11:58:20Z"}
		},
		pressure: func() pressureSample {
			return pressureSample{cpuAvg10: 1, memAvg10: 2, ioAvg10: 3}
		},
		numCPU:       func() int { return 4 },
		numGoroutine: func() int { return 8 },
		readMemStats: func(m *runtime.MemStats) {
			m.Alloc = 1024 * 1024
			m.Sys = 2 * 1024 * 1024
			m.HeapObjects = 10
			m.NumGC = 1
			m.PauseNs[0] = 5_000_000
		},
	}
}
