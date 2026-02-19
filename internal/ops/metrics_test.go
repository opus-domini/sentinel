package ops

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
