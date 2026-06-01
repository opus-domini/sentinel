package services

import (
	"context"
	"runtime"
	"sync"
	"time"
)

const (
	metricsSnapshotReuseInterval = time.Second
	metricsDiskInterval          = 10 * time.Second
	metricsProcessInterval       = 10 * time.Second
	metricsUptimeInterval        = 30 * time.Second
	metricsPressureInterval      = 10 * time.Second
)

// HostMetrics holds a snapshot of host resource metrics.
type HostMetrics struct {
	CPUPercent        float64 `json:"cpuPercent"`
	CPUCount          int     `json:"cpuCount"`
	LoadAvg1          float64 `json:"loadAvg1"`
	LoadAvg5          float64 `json:"loadAvg5"`
	LoadAvg15         float64 `json:"loadAvg15"`
	LoadPerCPU        float64 `json:"loadPerCPU"`
	MemUsedBytes      int64   `json:"memUsedBytes"`
	MemTotalBytes     int64   `json:"memTotalBytes"`
	MemAvailableBytes int64   `json:"memAvailableBytes"`
	MemPercent        float64 `json:"memPercent"`
	SwapUsedBytes     int64   `json:"swapUsedBytes"`
	SwapTotalBytes    int64   `json:"swapTotalBytes"`
	SwapPercent       float64 `json:"swapPercent"`
	DiskUsedBytes     int64   `json:"diskUsedBytes"`
	DiskTotalBytes    int64   `json:"diskTotalBytes"`
	DiskFreeBytes     int64   `json:"diskFreeBytes"`
	DiskPercent       float64 `json:"diskPercent"`
	DiskInodesUsed    int64   `json:"diskInodesUsed"`
	DiskInodesTotal   int64   `json:"diskInodesTotal"`
	DiskInodesPercent float64 `json:"diskInodesPercent"`
	NetRxBytes        int64   `json:"netRxBytes"`
	NetTxBytes        int64   `json:"netTxBytes"`
	NetInterfaces     int     `json:"netInterfaces"`
	ProcessCount      int     `json:"processCount"`
	ThreadCount       int     `json:"threadCount"`
	HostUptimeSec     int64   `json:"hostUptimeSec"`
	BootTime          string  `json:"bootTime"`
	CPUPressureAvg10  float64 `json:"cpuPressureAvg10"`
	MemPressureAvg10  float64 `json:"memPressureAvg10"`
	IOPressureAvg10   float64 `json:"ioPressureAvg10"`
	NumGoroutines     int     `json:"numGoroutines"`
	GoMemAllocMB      float64 `json:"goMemAllocMB"`
	GoMemSysMB        float64 `json:"goMemSysMB"`
	GoHeapObjects     uint64  `json:"goHeapObjects"`
	GoNumGC           uint32  `json:"goNumGC"`
	GoLastGCPauseMs   float64 `json:"goLastGcPauseMs"`
	CollectedAt       string  `json:"collectedAt"`
}

type memorySample struct {
	usedBytes      int64
	totalBytes     int64
	availableBytes int64
	swapUsedBytes  int64
	swapTotalBytes int64
}

type diskSample struct {
	usedBytes   int64
	totalBytes  int64
	freeBytes   int64
	inodesUsed  int64
	inodesTotal int64
}

type networkIOSample struct {
	rxBytes    int64
	txBytes    int64
	interfaces int
}

type processSample struct {
	processes int
	threads   int
	complete  bool
}

type uptimeSample struct {
	uptimeSec int64
	bootTime  string
}

type pressureSample struct {
	cpuAvg10 float64
	memAvg10 float64
	ioAvg10  float64
}

type metricsCollectionIntervals struct {
	snapshotReuse time.Duration
	disk          time.Duration
	process       time.Duration
	uptime        time.Duration
	pressure      time.Duration
}

type metricCollectors struct {
	cpuPercent   func(context.Context) float64
	memInfo      func(context.Context) memorySample
	loadAvg      func(context.Context) (float64, float64, float64)
	diskUsage    func(string) diskSample
	networkIO    func() networkIOSample
	processInfo  func(context.Context) processSample
	hostUptime   func() uptimeSample
	pressure     func() pressureSample
	numCPU       func() int
	numGoroutine func() int
	readMemStats func(*runtime.MemStats)
}

type metricsCollector struct {
	mu         sync.Mutex
	nowFn      func() time.Time
	intervals  metricsCollectionIntervals
	collectors metricCollectors

	hasSnapshot bool
	snapshot    HostMetrics
	snapshotAt  time.Time

	hasDisk  bool
	disk     diskSample
	diskAt   time.Time
	diskPath string

	hasProcess bool
	process    processSample
	processAt  time.Time

	hasUptime bool
	uptime    uptimeSample
	uptimeAt  time.Time

	hasPressure bool
	pressure    pressureSample
	pressureAt  time.Time
}

func newMetricsCollector() *metricsCollector {
	return newMetricsCollectorWith(time.Now, defaultMetricsCollectionIntervals(), defaultMetricCollectors())
}

func newMetricsCollectorWith(nowFn func() time.Time, intervals metricsCollectionIntervals, collectors metricCollectors) *metricsCollector {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &metricsCollector{
		nowFn:      nowFn,
		intervals:  intervals.withDefaults(),
		collectors: collectors.withDefaults(),
	}
}

// CollectMetrics gathers host resource metrics. diskPath is the filesystem
// path to stat for disk usage (defaults to "/" if empty).
func CollectMetrics(ctx context.Context, diskPath string) HostMetrics {
	return newMetricsCollector().Collect(ctx, diskPath)
}

func (c *metricsCollector) Collect(ctx context.Context, diskPath string) HostMetrics {
	if c == nil {
		return CollectMetrics(ctx, diskPath)
	}
	if diskPath == "" {
		diskPath = "/"
	}
	now := c.nowFn().UTC()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.hasSnapshot && reusableAt(now, c.snapshotAt, c.intervals.snapshotReuse) {
		return c.snapshot
	}

	cpuCount := c.collectors.numCPU()
	cpuPct := c.collectors.cpuPercent(ctx)
	mem := c.collectors.memInfo(ctx)
	avg1, avg5, avg15 := c.collectors.loadAvg(ctx)
	disk := c.diskLocked(diskPath, now)
	net := c.collectors.networkIO()
	processes := c.processLocked(ctx, now)
	uptime := c.uptimeLocked(now)
	pressure := c.pressureLocked(now)

	var memPct float64
	if mem.totalBytes > 0 {
		memPct = float64(mem.usedBytes) / float64(mem.totalBytes) * 100
	}

	var swapPct float64
	if mem.swapTotalBytes > 0 {
		swapPct = float64(mem.swapUsedBytes) / float64(mem.swapTotalBytes) * 100
	}

	var diskPct float64
	if disk.totalBytes > 0 {
		diskPct = float64(disk.usedBytes) / float64(disk.totalBytes) * 100
	}

	var diskInodesPct float64
	if disk.inodesTotal > 0 {
		diskInodesPct = float64(disk.inodesUsed) / float64(disk.inodesTotal) * 100
	}

	loadPerCPU := -1.0
	if avg1 >= 0 && cpuCount > 0 {
		loadPerCPU = avg1 / float64(cpuCount)
	}

	var memStats runtime.MemStats
	c.collectors.readMemStats(&memStats)
	lastGCPauseMS := 0.0
	if memStats.NumGC > 0 {
		lastGCPauseMS = float64(memStats.PauseNs[(memStats.NumGC+255)%256]) / 1_000_000
	}

	metrics := HostMetrics{
		CPUPercent:        cpuPct,
		CPUCount:          cpuCount,
		LoadAvg1:          avg1,
		LoadAvg5:          avg5,
		LoadAvg15:         avg15,
		LoadPerCPU:        loadPerCPU,
		MemUsedBytes:      mem.usedBytes,
		MemTotalBytes:     mem.totalBytes,
		MemAvailableBytes: mem.availableBytes,
		MemPercent:        memPct,
		SwapUsedBytes:     mem.swapUsedBytes,
		SwapTotalBytes:    mem.swapTotalBytes,
		SwapPercent:       swapPct,
		DiskUsedBytes:     disk.usedBytes,
		DiskTotalBytes:    disk.totalBytes,
		DiskFreeBytes:     disk.freeBytes,
		DiskPercent:       diskPct,
		DiskInodesUsed:    disk.inodesUsed,
		DiskInodesTotal:   disk.inodesTotal,
		DiskInodesPercent: diskInodesPct,
		NetRxBytes:        net.rxBytes,
		NetTxBytes:        net.txBytes,
		NetInterfaces:     net.interfaces,
		ProcessCount:      processes.processes,
		ThreadCount:       processes.threads,
		HostUptimeSec:     uptime.uptimeSec,
		BootTime:          uptime.bootTime,
		CPUPressureAvg10:  pressure.cpuAvg10,
		MemPressureAvg10:  pressure.memAvg10,
		IOPressureAvg10:   pressure.ioAvg10,
		NumGoroutines:     c.collectors.numGoroutine(),
		GoMemAllocMB:      float64(memStats.Alloc) / (1024 * 1024),
		GoMemSysMB:        float64(memStats.Sys) / (1024 * 1024),
		GoHeapObjects:     memStats.HeapObjects,
		GoNumGC:           memStats.NumGC,
		GoLastGCPauseMs:   lastGCPauseMS,
		CollectedAt:       now.Format(time.RFC3339),
	}

	c.snapshot = metrics
	c.snapshotAt = now
	c.hasSnapshot = true

	return metrics
}

func (c *metricsCollector) diskLocked(path string, now time.Time) diskSample {
	if c.hasDisk && c.diskPath == path && reusableAt(now, c.diskAt, c.intervals.disk) {
		return c.disk
	}

	c.disk = c.collectors.diskUsage(path)
	c.diskAt = now
	c.diskPath = path
	c.hasDisk = true
	return c.disk
}

func (c *metricsCollector) processLocked(ctx context.Context, now time.Time) processSample {
	if c.hasProcess && reusableAt(now, c.processAt, c.intervals.process) {
		return c.process
	}

	sample := c.collectors.processInfo(ctx)
	if !sample.complete && c.hasProcess {
		return c.process
	}
	c.process = sample
	c.processAt = now
	c.hasProcess = true
	return c.process
}

func (c *metricsCollector) uptimeLocked(now time.Time) uptimeSample {
	if c.hasUptime && reusableAt(now, c.uptimeAt, c.intervals.uptime) {
		return c.uptime
	}

	c.uptime = c.collectors.hostUptime()
	c.uptimeAt = now
	c.hasUptime = true
	return c.uptime
}

func (c *metricsCollector) pressureLocked(now time.Time) pressureSample {
	if c.hasPressure && reusableAt(now, c.pressureAt, c.intervals.pressure) {
		return c.pressure
	}

	c.pressure = c.collectors.pressure()
	c.pressureAt = now
	c.hasPressure = true
	return c.pressure
}

func reusableAt(now, collectedAt time.Time, interval time.Duration) bool {
	if interval <= 0 || collectedAt.IsZero() {
		return false
	}
	age := now.Sub(collectedAt)
	return age >= 0 && age < interval
}

func defaultMetricsCollectionIntervals() metricsCollectionIntervals {
	return metricsCollectionIntervals{
		snapshotReuse: metricsSnapshotReuseInterval,
		disk:          metricsDiskInterval,
		process:       metricsProcessInterval,
		uptime:        metricsUptimeInterval,
		pressure:      metricsPressureInterval,
	}
}

func (i metricsCollectionIntervals) withDefaults() metricsCollectionIntervals {
	defaults := defaultMetricsCollectionIntervals()
	if i.snapshotReuse == 0 {
		i.snapshotReuse = defaults.snapshotReuse
	}
	if i.disk == 0 {
		i.disk = defaults.disk
	}
	if i.process == 0 {
		i.process = defaults.process
	}
	if i.uptime == 0 {
		i.uptime = defaults.uptime
	}
	if i.pressure == 0 {
		i.pressure = defaults.pressure
	}
	return i
}

func defaultMetricCollectors() metricCollectors {
	return metricCollectors{
		cpuPercent:   collectCPUPercent,
		memInfo:      collectMemInfo,
		loadAvg:      collectLoadAvg,
		diskUsage:    collectDiskUsage,
		networkIO:    collectNetworkIO,
		processInfo:  collectProcessInfo,
		hostUptime:   collectHostUptime,
		pressure:     collectPressure,
		numCPU:       runtime.NumCPU,
		numGoroutine: runtime.NumGoroutine,
		readMemStats: runtime.ReadMemStats,
	}
}

func (c metricCollectors) withDefaults() metricCollectors {
	defaults := defaultMetricCollectors()
	if c.cpuPercent == nil {
		c.cpuPercent = defaults.cpuPercent
	}
	if c.memInfo == nil {
		c.memInfo = defaults.memInfo
	}
	if c.loadAvg == nil {
		c.loadAvg = defaults.loadAvg
	}
	if c.diskUsage == nil {
		c.diskUsage = defaults.diskUsage
	}
	if c.networkIO == nil {
		c.networkIO = defaults.networkIO
	}
	if c.processInfo == nil {
		c.processInfo = defaults.processInfo
	}
	if c.hostUptime == nil {
		c.hostUptime = defaults.hostUptime
	}
	if c.pressure == nil {
		c.pressure = defaults.pressure
	}
	if c.numCPU == nil {
		c.numCPU = defaults.numCPU
	}
	if c.numGoroutine == nil {
		c.numGoroutine = defaults.numGoroutine
	}
	if c.readMemStats == nil {
		c.readMemStats = defaults.readMemStats
	}
	return c
}
