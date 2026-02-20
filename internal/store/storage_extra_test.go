package store

import (
	"context"
	"testing"
)

func TestIsStorageResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"timeline", StorageResourceTimeline, true},
		{"activity_log", StorageResourceActivityLog, true},
		{"guardrail_log", StorageResourceGuardrailLog, true},
		{"recovery_log", StorageResourceRecoveryLog, true},
		{"ops_activity", StorageResourceOpsActivity, true},
		{"ops_alerts", StorageResourceOpsAlerts, true},
		{"ops_jobs", StorageResourceOpsJobs, true},
		{"all", StorageResourceAll, true},
		{"uppercase_timeline", "TIMELINE", true},
		{"mixed_case", "Timeline", true},
		{"with_spaces", "  timeline  ", true},

		{"empty", "", false},
		{"unknown", "unknown-resource", false},
		{"partial", "time", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := IsStorageResource(tt.input)
			if got != tt.want {
				t.Errorf("IsStorageResource(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeStorageResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"Timeline", "timeline"},
		{"  ACTIVITY-JOURNAL  ", "activity-journal"},
		{"all", "all"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := NormalizeStorageResource(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeStorageResource(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFlushSingleResource(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	// Flush a single resource (should succeed even when empty).
	results, err := s.FlushStorageResource(ctx, StorageResourceTimeline)
	if err != nil {
		t.Fatalf("FlushStorageResource(timeline): %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Resource != StorageResourceTimeline {
		t.Fatalf("resource = %q, want %q", results[0].Resource, StorageResourceTimeline)
	}
}

func TestWatchtowerGlobalRevision(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	// No revision yet.
	rev, err := s.WatchtowerGlobalRevision(ctx)
	if err != nil {
		t.Fatalf("WatchtowerGlobalRevision: %v", err)
	}
	if rev != 0 {
		t.Fatalf("rev = %d, want 0", rev)
	}

	// Set a value and check.
	if err := s.SetWatchtowerRuntimeValue(ctx, "global_rev", "42"); err != nil {
		t.Fatalf("SetWatchtowerRuntimeValue: %v", err)
	}
	rev, err = s.WatchtowerGlobalRevision(ctx)
	if err != nil {
		t.Fatalf("WatchtowerGlobalRevision: %v", err)
	}
	if rev != 42 {
		t.Fatalf("rev = %d, want 42", rev)
	}
}
