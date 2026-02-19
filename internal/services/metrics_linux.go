//go:build linux

package services

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func collectCPUPercent(ctx context.Context) float64 {
	idle1, total1, err := readCPUStat()
	if err != nil {
		return -1
	}

	select {
	case <-ctx.Done():
		return -1
	case <-time.After(100 * time.Millisecond):
	}

	idle2, total2, err := readCPUStat()
	if err != nil {
		return -1
	}

	totalDelta := total2 - total1
	idleDelta := idle2 - idle1
	if totalDelta <= 0 {
		return 0
	}
	return float64(totalDelta-idleDelta) / float64(totalDelta) * 100
}

// readCPUStat reads /proc/stat and returns (idle, total) CPU time values.
func readCPUStat() (idle, total uint64, err error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return 0, 0, fmt.Errorf("unexpected /proc/stat cpu line: %s", line)
		}
		// fields: cpu user nice system idle ...
		var sum uint64
		for i := 1; i < len(fields); i++ {
			v, parseErr := strconv.ParseUint(fields[i], 10, 64)
			if parseErr != nil {
				continue
			}
			sum += v
			if i == 4 {
				idle = v
			}
		}
		return idle, sum, nil
	}
	return 0, 0, fmt.Errorf("cpu line not found in /proc/stat")
}

func collectMemInfo() (used, total int64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}

	var memTotal, memAvailable, memFree, buffers, cached int64
	foundAvailable := false

	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		val, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		// Values in /proc/meminfo are in kB.
		valBytes := val * 1024
		switch parts[0] {
		case "MemTotal:":
			memTotal = valBytes
		case "MemAvailable:":
			memAvailable = valBytes
			foundAvailable = true
		case "MemFree:":
			memFree = valBytes
		case "Buffers:":
			buffers = valBytes
		case "Cached:":
			cached = valBytes
		}
	}

	total = memTotal
	if foundAvailable {
		used = memTotal - memAvailable
	} else {
		used = memTotal - (memFree + buffers + cached)
	}
	if used < 0 {
		used = 0
	}
	return used, total
}

func collectLoadAvg() (avg1, avg5, avg15 float64) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return -1, -1, -1
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return -1, -1, -1
	}
	avg1, _ = strconv.ParseFloat(fields[0], 64)
	avg5, _ = strconv.ParseFloat(fields[1], 64)
	avg15, _ = strconv.ParseFloat(fields[2], 64)
	return avg1, avg5, avg15
}

func collectDiskUsage(path string) (used, total int64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0
	}
	// Total = blocks * block size, Available = free blocks available to unprivileged users.
	bsize := uint64(stat.Bsize)
	total = int64(stat.Blocks * bsize)
	free := int64(stat.Bavail * bsize)
	used = total - free
	if used < 0 {
		used = 0
	}
	return used, total
}
