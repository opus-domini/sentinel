//go:build darwin

package ops

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func collectCPUPercent(_ context.Context) float64 {
	// macOS does not have /proc/stat. Use top -l 1 to get a snapshot of
	// CPU usage, parsing the idle percentage from its summary line.
	out, err := exec.Command("top", "-l", "1", "-n", "0", "-s", "0").Output()
	if err != nil {
		return -1
	}

	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "CPU usage:") {
			continue
		}
		// Line format: "CPU usage: 3.33% user, 6.66% sys, 90.0% idle"
		parts := strings.Split(line, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasSuffix(part, "idle") {
				pctStr := strings.TrimSuffix(part, "idle")
				pctStr = strings.TrimSuffix(strings.TrimSpace(pctStr), "%")
				pctStr = strings.TrimSpace(pctStr)
				idle, parseErr := strconv.ParseFloat(pctStr, 64)
				if parseErr == nil {
					return 100 - idle
				}
			}
		}
	}

	return -1
}

func collectMemInfo() (used, total int64) {
	// Use sysctl to get physical memory size.
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, 0
	}
	total, err = strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0, 0
	}

	// Get page size and VM stats for used memory approximation.
	pageOut, err := exec.Command("sysctl", "-n", "hw.pagesize").Output()
	if err != nil {
		return 0, total
	}
	pageSize, err := strconv.ParseInt(strings.TrimSpace(string(pageOut)), 10, 64)
	if err != nil {
		return 0, total
	}

	vmOut, err := exec.Command("vm_stat").Output()
	if err != nil {
		return 0, total
	}

	pageValues := make(map[string]int64)
	for _, line := range strings.Split(string(vmOut), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		valStr := strings.TrimSpace(parts[1])
		valStr = strings.TrimSuffix(valStr, ".")
		val, parseErr := strconv.ParseInt(valStr, 10, 64)
		if parseErr != nil {
			continue
		}
		pageValues[strings.TrimSpace(parts[0])] = val
	}

	// Active + wired + compressed = used memory approximation.
	activePages := pageValues["Pages active"]
	wiredPages := pageValues["Pages wired down"]
	compressedPages := pageValues["Pages occupied by compressor"]
	used = (activePages + wiredPages + compressedPages) * pageSize
	if used < 0 {
		used = 0
	}
	return used, total
}

func collectLoadAvg() (avg1, avg5, avg15 float64) {
	out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
	if err != nil {
		return -1, -1, -1
	}
	// Output format: "{ 1.23 4.56 7.89 }"
	trimmed := strings.TrimSpace(string(out))
	trimmed = strings.TrimPrefix(trimmed, "{")
	trimmed = strings.TrimSuffix(trimmed, "}")
	fields := strings.Fields(trimmed)
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
	bsize := uint64(stat.Bsize)
	total = int64(stat.Blocks * bsize)
	free := int64(stat.Bavail * bsize)
	used = total - free
	if used < 0 {
		used = 0
	}
	return used, total
}
