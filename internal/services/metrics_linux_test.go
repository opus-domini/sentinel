//go:build linux

package services

import "testing"

// Not parallel: procRootPath is package state shared with every collector.
func TestReadCPUStatExcludesGuestFromTotal(t *testing.T) {
	root := t.TempDir()
	writeProcFixture(t, root, "stat", "cpu  100 20 50 800 0 0 0 0 30 10\nbtime 1760000000\n")

	originalProcRoot := procRootPath
	t.Cleanup(func() { procRootPath = originalProcRoot })
	procRootPath = root

	idle, total, err := readCPUStat()
	if err != nil {
		t.Fatalf("readCPUStat() error = %v", err)
	}
	if idle != 800 {
		t.Fatalf("idle = %d, want 800", idle)
	}
	// user+nice+system+idle+iowait+irq+softirq+steal = 970. guest (30) and
	// guest_nice (10) are already accounted inside user and nice.
	if total != 970 {
		t.Fatalf("total = %d, want 970 (guest and guest_nice counted twice)", total)
	}
}

func TestReadCPUStatRejectsUnparsableField(t *testing.T) {
	root := t.TempDir()
	writeProcFixture(t, root, "stat", "cpu  100 20 notanumber 800 0\n")

	originalProcRoot := procRootPath
	t.Cleanup(func() { procRootPath = originalProcRoot })
	procRootPath = root

	if _, _, err := readCPUStat(); err == nil {
		t.Fatal("readCPUStat() error = nil, want error for unparsable cpu field")
	}
}
