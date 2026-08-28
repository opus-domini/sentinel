//go:build !linux && !darwin

package services

import (
	"context"
	"runtime"
	"time"
)

func collectCPUPercent(_ context.Context) float64 {
	// No reliable cross-platform CPU metric without cgo or external deps.
	_ = runtime.NumCPU()
	return -1
}

func collectMemInfo() memorySample {
	// Use Go runtime as a rough approximation on unsupported platforms.
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return memorySample{
		usedBytes:      int64(m.Sys),
		totalBytes:     int64(m.Sys),
		availableBytes: 0,
	}
}

func collectLoadAvg() (avg1, avg5, avg15 float64) {
	return -1, -1, -1
}

func collectDiskUsage(_ string) diskSample {
	return diskSample{}
}

func collectNetworkIO() networkIOSample {
	return networkIOSample{}
}

func collectProcessInfo(_ context.Context) processSample {
	return processSample{complete: true}
}

func collectHostUptime() uptimeSample {
	return uptimeSample{}
}

func collectPressure() pressureSample {
	return pressureSample{cpuAvg10: -1, memAvg10: -1, ioAvg10: -1}
}

func newSensorCollector() func(time.Time) SensorMetrics {
	return func(time.Time) SensorMetrics { return emptySensorMetrics() }
}
