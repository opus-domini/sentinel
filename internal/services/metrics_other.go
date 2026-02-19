//go:build !linux && !darwin

package services

import (
	"context"
	"runtime"
)

func collectCPUPercent(_ context.Context) float64 {
	// No reliable cross-platform CPU metric without cgo or external deps.
	_ = runtime.NumCPU()
	return -1
}

func collectMemInfo() (used, total int64) {
	// Use Go runtime as a rough approximation on unsupported platforms.
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return int64(m.Sys), int64(m.Sys)
}

func collectLoadAvg() (avg1, avg5, avg15 float64) {
	return -1, -1, -1
}

func collectDiskUsage(_ string) (used, total int64) {
	return 0, 0
}
