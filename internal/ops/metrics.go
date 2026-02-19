package ops

import (
	"context"
	"runtime"
	"time"
)

// HostMetrics holds a snapshot of host resource metrics.
type HostMetrics struct {
	CPUPercent     float64 `json:"cpuPercent"`
	MemUsedBytes   int64   `json:"memUsedBytes"`
	MemTotalBytes  int64   `json:"memTotalBytes"`
	MemPercent     float64 `json:"memPercent"`
	DiskUsedBytes  int64   `json:"diskUsedBytes"`
	DiskTotalBytes int64   `json:"diskTotalBytes"`
	DiskPercent    float64 `json:"diskPercent"`
	LoadAvg1       float64 `json:"loadAvg1"`
	LoadAvg5       float64 `json:"loadAvg5"`
	LoadAvg15      float64 `json:"loadAvg15"`
	NumGoroutines  int     `json:"numGoroutines"`
	GoMemAllocMB   float64 `json:"goMemAllocMB"`
	CollectedAt    string  `json:"collectedAt"`
}

// CollectMetrics gathers host resource metrics. diskPath is the filesystem
// path to stat for disk usage (defaults to "/" if empty).
func CollectMetrics(ctx context.Context, diskPath string) HostMetrics {
	if diskPath == "" {
		diskPath = "/"
	}

	cpuPct := collectCPUPercent(ctx)
	memUsed, memTotal := collectMemInfo()
	avg1, avg5, avg15 := collectLoadAvg()
	diskUsed, diskTotal := collectDiskUsage(diskPath)

	var memPct float64
	if memTotal > 0 {
		memPct = float64(memUsed) / float64(memTotal) * 100
	}

	var diskPct float64
	if diskTotal > 0 {
		diskPct = float64(diskUsed) / float64(diskTotal) * 100
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return HostMetrics{
		CPUPercent:     cpuPct,
		MemUsedBytes:   memUsed,
		MemTotalBytes:  memTotal,
		MemPercent:     memPct,
		DiskUsedBytes:  diskUsed,
		DiskTotalBytes: diskTotal,
		DiskPercent:    diskPct,
		LoadAvg1:       avg1,
		LoadAvg5:       avg5,
		LoadAvg15:      avg15,
		NumGoroutines:  runtime.NumGoroutine(),
		GoMemAllocMB:   float64(memStats.Alloc) / (1024 * 1024),
		CollectedAt:    time.Now().UTC().Format(time.RFC3339),
	}
}
