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

func collectMemInfo() memorySample {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return memorySample{}
	}

	var memTotal, memAvailable, memFree, buffers, cached, swapTotal, swapFree int64
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
		case "SwapTotal:":
			swapTotal = valBytes
		case "SwapFree:":
			swapFree = valBytes
		}
	}

	used := int64(0)
	available := int64(0)
	if foundAvailable {
		memAvailable = maxInt64(memAvailable, 0)
		used = memTotal - memAvailable
		available = memAvailable
	} else {
		available = memFree + buffers + cached
		used = memTotal - available
	}
	if used < 0 {
		used = 0
	}
	swapUsed := swapTotal - swapFree
	if swapUsed < 0 {
		swapUsed = 0
	}
	return memorySample{
		usedBytes:      used,
		totalBytes:     memTotal,
		availableBytes: available,
		swapUsedBytes:  swapUsed,
		swapTotalBytes: swapTotal,
	}
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

func collectDiskUsage(path string) diskSample {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return diskSample{}
	}
	// Total = blocks * block size, Available = free blocks available to unprivileged users.
	bsize := uint64(stat.Bsize)
	total := int64(stat.Blocks * bsize)
	free := int64(stat.Bavail * bsize)
	used := total - free
	if used < 0 {
		used = 0
	}
	inodesTotal := int64(stat.Files)
	inodesFree := int64(stat.Ffree)
	inodesUsed := inodesTotal - inodesFree
	if inodesUsed < 0 {
		inodesUsed = 0
	}
	return diskSample{
		usedBytes:   used,
		totalBytes:  total,
		freeBytes:   free,
		inodesUsed:  inodesUsed,
		inodesTotal: inodesTotal,
	}
}

func collectNetworkIO() networkIOSample {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return networkIOSample{}
	}

	var sample networkIOSample
	for _, line := range strings.Split(string(data), "\n") {
		namePart, countersPart, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name := strings.TrimSpace(namePart)
		if name == "" || name == "lo" {
			continue
		}
		fields := strings.Fields(countersPart)
		if len(fields) < 16 {
			continue
		}
		rx, rxErr := strconv.ParseInt(fields[0], 10, 64)
		tx, txErr := strconv.ParseInt(fields[8], 10, 64)
		if rxErr != nil || txErr != nil {
			continue
		}
		sample.rxBytes += rx
		sample.txBytes += tx
		sample.interfaces++
	}
	return sample
}

func collectProcessInfo(ctx context.Context) processSample {
	procRoot, err := os.OpenRoot("/proc")
	if err != nil {
		return processSample{complete: true}
	}
	defer func() {
		_ = procRoot.Close()
	}()

	dir, err := procRoot.Open(".")
	if err != nil {
		return processSample{complete: true}
	}
	defer func() {
		_ = dir.Close()
	}()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return processSample{complete: true}
	}

	sample := processSample{complete: true}
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			sample.complete = false
			return sample
		default:
		}
		if !entry.IsDir() || !isNumeric(entry.Name()) {
			continue
		}
		sample.processes++
		threads := readProcThreads(ctx, procRoot, entry.Name())
		if ctx.Err() != nil {
			sample.complete = false
			return sample
		}
		if threads > 0 {
			sample.threads += threads
		} else {
			sample.threads++
		}
	}
	return sample
}

func readProcThreads(ctx context.Context, procRoot *os.Root, pid string) int {
	select {
	case <-ctx.Done():
		return 0
	default:
	}
	if !isNumeric(pid) {
		return 0
	}

	data, err := procRoot.ReadFile(pid + "/status")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "Threads:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		threads, err := strconv.Atoi(fields[1])
		if err != nil {
			return 0
		}
		return threads
	}
	return 0
}

func collectHostUptime() uptimeSample {
	uptime := uptimeSample{}
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			if seconds, parseErr := strconv.ParseFloat(fields[0], 64); parseErr == nil {
				uptime.uptimeSec = int64(seconds)
			}
		}
	}
	if data, err := os.ReadFile("/proc/stat"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) != 2 || fields[0] != "btime" {
				continue
			}
			if seconds, parseErr := strconv.ParseInt(fields[1], 10, 64); parseErr == nil {
				uptime.bootTime = time.Unix(seconds, 0).UTC().Format(time.RFC3339)
			}
			break
		}
	}
	return uptime
}

func collectPressure() pressureSample {
	return pressureSample{
		cpuAvg10: readPressureAvg10(pressureCPU),
		memAvg10: readPressureAvg10(pressureMemory),
		ioAvg10:  readPressureAvg10(pressureIO),
	}
}

type pressureSource int

const (
	pressureCPU pressureSource = iota
	pressureMemory
	pressureIO
)

func readPressureAvg10(source pressureSource) float64 {
	var data []byte
	var err error
	switch source {
	case pressureCPU:
		data, err = os.ReadFile("/proc/pressure/cpu")
	case pressureMemory:
		data, err = os.ReadFile("/proc/pressure/memory")
	case pressureIO:
		data, err = os.ReadFile("/proc/pressure/io")
	default:
		return -1
	}
	if err != nil {
		return -1
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "some" {
			continue
		}
		for _, field := range fields[1:] {
			raw, ok := strings.CutPrefix(field, "avg10=")
			if !ok {
				continue
			}
			value, parseErr := strconv.ParseFloat(raw, 64)
			if parseErr != nil {
				return -1
			}
			return value
		}
	}
	return -1
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
