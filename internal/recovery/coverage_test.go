package recovery

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/alerts"
	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

const (
	testBootOverview = "boot-overview"
	testSessionDev   = "dev"
	testCdPrefix     = "cd "
	testCmdNvim      = "nvim"
)

// ---------------------------------------------------------------------------
// shellQuoteSingle
// ---------------------------------------------------------------------------

func TestShellQuoteSingle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", "''"},
		{"whitespace_only", "   ", "''"},
		{"simple_path", "/tmp/dev", "'/tmp/dev'"},
		{"with_single_quote", "it's", `'it'\''s'`},
		{"with_spaces", "/home/user/my dir", "'/home/user/my dir'"},
		{"multiple_quotes", "a'b'c", `'a'\''b'\''c'`},
		{"leading_trailing_spaces", "  /tmp  ", "'/tmp'"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := shellQuoteSingle(tc.input)
			if got != tc.want {
				t.Fatalf("shellQuoteSingle(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseOverviewTime
// ---------------------------------------------------------------------------

func TestParseOverviewTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		isZero bool
	}{
		{"empty", "", true},
		{"whitespace", "   ", true},
		{"invalid", "not-a-time", true},
		{"valid_rfc3339", "2024-01-15T10:30:00Z", false},
		{"valid_with_offset", "2024-01-15T10:30:00+02:00", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseOverviewTime(tc.input)
			if tc.isZero && !got.IsZero() {
				t.Fatalf("parseOverviewTime(%q) = %v, want zero", tc.input, got)
			}
			if !tc.isZero && got.IsZero() {
				t.Fatalf("parseOverviewTime(%q) = zero, want non-zero", tc.input)
			}
			if !tc.isZero && got.Location() != time.UTC {
				t.Fatalf("parseOverviewTime(%q) location = %v, want UTC", tc.input, got.Location())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// paneCWDByIndex
// ---------------------------------------------------------------------------

func TestPaneCWDByIndex(t *testing.T) {
	t.Parallel()

	panes := []PaneSnapshot{
		{PaneIndex: 0, CurrentPath: "/home/user"},
		{PaneIndex: 1, CurrentPath: "  /tmp  "},
		{PaneIndex: 3, CurrentPath: "/var/log"},
	}

	tests := []struct {
		name  string
		index int
		want  string
	}{
		{"found_0", 0, "/home/user"},
		{"found_1_trimmed", 1, "/tmp"},
		{"found_3", 3, "/var/log"},
		{"not_found", 2, ""},
		{"negative", -1, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := paneCWDByIndex(panes, tc.index)
			if got != tc.want {
				t.Fatalf("paneCWDByIndex(_, %d) = %q, want %q", tc.index, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// estimateTotalSteps
// ---------------------------------------------------------------------------

func TestEstimateTotalSteps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		snap SessionSnapshot
		want int
	}{
		{
			"empty_snapshot",
			SessionSnapshot{},
			1,
		},
		{
			"one_window_one_pane",
			SessionSnapshot{
				Windows: []WindowSnapshot{{Index: 0}},
				Panes:   []PaneSnapshot{{WindowIndex: 0}},
			},
			4, // 1 + 1*2 + 1
		},
		{
			"two_windows_three_panes",
			SessionSnapshot{
				Windows: []WindowSnapshot{{Index: 0}, {Index: 1}},
				Panes:   []PaneSnapshot{{WindowIndex: 0}, {WindowIndex: 0}, {WindowIndex: 1}},
			},
			8, // 1 + 2*2 + 3
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := estimateTotalSteps(tc.snap)
			if got != tc.want {
				t.Fatalf("estimateTotalSteps() = %d, want %d", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// normalize (RestoreOptions)
// ---------------------------------------------------------------------------

func TestRestoreOptionsNormalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      RestoreOptions
		wantMode   ReplayMode
		wantPolicy ConflictPolicy
	}{
		{"defaults_for_unknown", RestoreOptions{Mode: "garbage", ConflictPolicy: "garbage"}, ReplayModeConfirm, ConflictRename},
		{"preserves_safe", RestoreOptions{Mode: ReplayModeSafe, ConflictPolicy: ConflictSkip}, ReplayModeSafe, ConflictSkip},
		{"preserves_full", RestoreOptions{Mode: ReplayModeFull, ConflictPolicy: ConflictReplace}, ReplayModeFull, ConflictReplace},
		{"empty_defaults", RestoreOptions{}, ReplayModeConfirm, ConflictRename},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.input.normalize()
			if got.Mode != tc.wantMode {
				t.Fatalf("mode = %q, want %q", got.Mode, tc.wantMode)
			}
			if got.ConflictPolicy != tc.wantPolicy {
				t.Fatalf("policy = %q, want %q", got.ConflictPolicy, tc.wantPolicy)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Overview
// ---------------------------------------------------------------------------

func TestOverview(t *testing.T) {
	t.Parallel()

	st := newRecoveryStore(t)
	ctx := context.Background()

	fake := &fakeTmux{
		sessions: []tmux.Session{
			{Name: "dev", Attached: 1, CreatedAt: time.Now().UTC(), ActivityAt: time.Now().UTC()},
		},
		windows: map[string][]tmux.Window{
			"dev": {{Session: "dev", Index: 0, Name: "main", Active: true, Panes: 1, Layout: "abc"}},
		},
		panes: map[string][]tmux.Pane{
			"dev": {{Session: "dev", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", Active: true, CurrentPath: "/tmp"}},
		},
	}

	svc := New(st, fake, Options{})
	svc.bootID = func(context.Context) string { return testBootOverview }

	if err := svc.Collect(ctx); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	overview, err := svc.Overview(ctx)
	if err != nil {
		t.Fatalf("Overview() error = %v", err)
	}
	if overview.BootID != testBootOverview {
		t.Fatalf("BootID = %q, want %s", overview.BootID, testBootOverview)
	}
	if overview.LastBootID != testBootOverview {
		t.Fatalf("LastBootID = %q, want %s", overview.LastBootID, testBootOverview)
	}
	if overview.LastCollectAt.IsZero() {
		t.Fatal("LastCollectAt is zero")
	}
}

// ---------------------------------------------------------------------------
// ListKilledSessions
// ---------------------------------------------------------------------------

func TestListKilledSessions(t *testing.T) {
	t.Parallel()

	st := newRecoveryStore(t)
	ctx := context.Background()

	fake := &fakeTmux{
		sessions: []tmux.Session{
			{Name: "work", Attached: 1, CreatedAt: time.Now().UTC(), ActivityAt: time.Now().UTC()},
		},
		windows: map[string][]tmux.Window{
			"work": {{Session: "work", Index: 0, Name: "main", Active: true, Panes: 1, Layout: "abc"}},
		},
		panes: map[string][]tmux.Pane{
			"work": {{Session: "work", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", Active: true}},
		},
	}

	bootID := "boot-1"
	svc := New(st, fake, Options{})
	svc.bootID = func(context.Context) string { return bootID }

	if err := svc.Collect(ctx); err != nil {
		t.Fatalf("first Collect() error = %v", err)
	}

	// Simulate boot change with no live sessions.
	bootID = "boot-2"
	fake.sessions = nil
	fake.windows = map[string][]tmux.Window{}
	fake.panes = map[string][]tmux.Pane{}

	if err := svc.Collect(ctx); err != nil {
		t.Fatalf("second Collect() error = %v", err)
	}

	killed, err := svc.ListKilledSessions(ctx)
	if err != nil {
		t.Fatalf("ListKilledSessions() error = %v", err)
	}
	if len(killed) != 1 {
		t.Fatalf("killed sessions = %d, want 1", len(killed))
	}
	if killed[0].Name != "work" {
		t.Fatalf("killed session = %q, want work", killed[0].Name)
	}
}

// ---------------------------------------------------------------------------
// ListSnapshots
// ---------------------------------------------------------------------------

func TestListSnapshots(t *testing.T) {
	t.Parallel()

	st := newRecoveryStore(t)
	ctx := context.Background()

	fake := &fakeTmux{
		sessions: []tmux.Session{
			{Name: "dev", Attached: 1, CreatedAt: time.Now().UTC(), ActivityAt: time.Now().UTC()},
		},
		windows: map[string][]tmux.Window{
			"dev": {{Session: "dev", Index: 0, Name: "main", Active: true, Panes: 1, Layout: "abc"}},
		},
		panes: map[string][]tmux.Pane{
			"dev": {{Session: "dev", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", Active: true, CurrentPath: "/tmp"}},
		},
	}

	svc := New(st, fake, Options{})
	svc.bootID = func(context.Context) string { return "boot-snap" }

	if err := svc.Collect(ctx); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	snapshots, err := svc.ListSnapshots(ctx, testSessionDev, 10)
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snapshots))
	}
	if snapshots[0].SessionName != testSessionDev {
		t.Fatalf("snapshot session = %q, want %s", snapshots[0].SessionName, testSessionDev)
	}
}

// ---------------------------------------------------------------------------
// GetJob
// ---------------------------------------------------------------------------

func TestGetJob(t *testing.T) {
	t.Parallel()

	st := newRecoveryStore(t)
	ctx := context.Background()

	// Create a job directly in the store.
	job := store.RecoveryJob{
		ID:             "test-job-1",
		SessionName:    "dev",
		TargetSession:  "dev",
		SnapshotID:     1,
		Mode:           "safe",
		ConflictPolicy: "rename",
		Status:         store.RecoveryJobQueued,
		TotalSteps:     5,
		CreatedAt:      time.Now().UTC(),
	}
	if err := st.CreateRecoveryJob(ctx, job); err != nil {
		t.Fatalf("CreateRecoveryJob() error = %v", err)
	}

	svc := New(st, &fakeTmux{}, Options{})

	got, err := svc.GetJob(ctx, "test-job-1")
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if got.ID != "test-job-1" {
		t.Fatalf("job ID = %q, want test-job-1", got.ID)
	}
	if got.Status != store.RecoveryJobQueued {
		t.Fatalf("job status = %q, want queued", got.Status)
	}
}

// ---------------------------------------------------------------------------
// ArchiveSession
// ---------------------------------------------------------------------------

type fakeAlertRepo struct {
	mu         sync.Mutex
	upserted   []alerts.AlertWrite
	resolved   []string
	resolveErr error
}

func (f *fakeAlertRepo) UpsertAlert(_ context.Context, write alerts.AlertWrite) (alerts.Alert, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upserted = append(f.upserted, write)
	return alerts.Alert{}, nil
}

func (f *fakeAlertRepo) ResolveAlert(_ context.Context, dedupeKey string, _ time.Time) (alerts.Alert, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolved = append(f.resolved, dedupeKey)
	return alerts.Alert{}, f.resolveErr
}

func TestArchiveSession(t *testing.T) {
	t.Parallel()

	st := newRecoveryStore(t)
	ctx := context.Background()

	fake := &fakeTmux{
		sessions: []tmux.Session{
			{Name: "arch", Attached: 1, CreatedAt: time.Now().UTC(), ActivityAt: time.Now().UTC()},
		},
		windows: map[string][]tmux.Window{
			"arch": {{Session: "arch", Index: 0, Name: "main", Active: true, Panes: 1, Layout: "abc"}},
		},
		panes: map[string][]tmux.Pane{
			"arch": {{Session: "arch", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", Active: true}},
		},
	}

	alertRepo := &fakeAlertRepo{}
	bootID := "boot-a"
	svc := New(st, fake, Options{AlertRepo: alertRepo})
	svc.bootID = func(context.Context) string { return bootID }

	// Collect to register the session.
	if err := svc.Collect(ctx); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	// Simulate boot change to mark as killed.
	bootID = "boot-b"
	fake.sessions = nil
	fake.windows = map[string][]tmux.Window{}
	fake.panes = map[string][]tmux.Pane{}
	if err := svc.Collect(ctx); err != nil {
		t.Fatalf("second Collect() error = %v", err)
	}

	// Now archive the session.
	if err := svc.ArchiveSession(ctx, "arch"); err != nil {
		t.Fatalf("ArchiveSession() error = %v", err)
	}

	// Verify it's no longer in killed list.
	killed, err := svc.ListKilledSessions(ctx)
	if err != nil {
		t.Fatalf("ListKilledSessions() error = %v", err)
	}
	for _, s := range killed {
		if s.Name == "arch" && s.State == store.RecoveryStateKilled {
			t.Fatal("session should be archived, not killed")
		}
	}

	// Verify alert was resolved.
	alertRepo.mu.Lock()
	defer alertRepo.mu.Unlock()
	found := false
	for _, key := range alertRepo.resolved {
		if key == "recovery:session:arch:killed" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected alert to be resolved for archived session")
	}
}

// ---------------------------------------------------------------------------
// resolveRestoreTarget — skip policy
// ---------------------------------------------------------------------------

func TestResolveRestoreTargetSkip(t *testing.T) {
	t.Parallel()

	fake := &fakeTmux{
		sessions: []tmux.Session{{Name: "existing"}},
	}
	svc := New(newRecoveryStore(t), fake, Options{})

	_, err := svc.resolveRestoreTarget(context.Background(), "existing", ConflictSkip)
	if err == nil {
		t.Fatal("expected error for skip policy when session exists")
	}
	if !errors.Is(err, err) { // just verify error is non-nil
		t.Fatalf("unexpected error type: %v", err)
	}
}

// ---------------------------------------------------------------------------
// resolveRestoreTarget — rename policy
// ---------------------------------------------------------------------------

func TestResolveRestoreTargetRename(t *testing.T) {
	t.Parallel()

	fake := &fakeTmux{
		sessions: []tmux.Session{{Name: "dev"}},
	}
	svc := New(newRecoveryStore(t), fake, Options{})

	target, err := svc.resolveRestoreTarget(context.Background(), "dev", ConflictRename)
	if err != nil {
		t.Fatalf("resolveRestoreTarget(rename) error = %v", err)
	}
	if target != "dev-restored-01" {
		t.Fatalf("target = %q, want dev-restored-01", target)
	}
}

// ---------------------------------------------------------------------------
// resolveRestoreTarget — replace policy
// ---------------------------------------------------------------------------

func TestResolveRestoreTargetReplace(t *testing.T) {
	t.Parallel()

	fake := &fakeTmux{
		sessions: []tmux.Session{{Name: "dev"}},
	}
	svc := New(newRecoveryStore(t), fake, Options{})

	target, err := svc.resolveRestoreTarget(context.Background(), "dev", ConflictReplace)
	if err != nil {
		t.Fatalf("resolveRestoreTarget(replace) error = %v", err)
	}
	if target != "dev" {
		t.Fatalf("target = %q, want dev", target)
	}
	// Verify the session was killed.
	for _, s := range fake.sessions {
		if s.Name == "dev" {
			t.Fatal("session should have been killed")
		}
	}
}

// ---------------------------------------------------------------------------
// resolveRestoreTarget — not exists
// ---------------------------------------------------------------------------

func TestResolveRestoreTargetNotExists(t *testing.T) {
	t.Parallel()

	fake := &fakeTmux{}
	svc := New(newRecoveryStore(t), fake, Options{})

	target, err := svc.resolveRestoreTarget(context.Background(), "new-session", ConflictRename)
	if err != nil {
		t.Fatalf("resolveRestoreTarget() error = %v", err)
	}
	if target != "new-session" {
		t.Fatalf("target = %q, want new-session", target)
	}
}

// ---------------------------------------------------------------------------
// resolveRestoreTarget — empty target
// ---------------------------------------------------------------------------

func TestResolveRestoreTargetEmptyTarget(t *testing.T) {
	t.Parallel()

	svc := New(newRecoveryStore(t), &fakeTmux{}, Options{})

	_, err := svc.resolveRestoreTarget(context.Background(), "", ConflictRename)
	if err == nil {
		t.Fatal("expected error for empty target")
	}
}

// ---------------------------------------------------------------------------
// resolveRestoreTarget — rename exhaustion
// ---------------------------------------------------------------------------

func TestResolveRestoreTargetRenameExhaustion(t *testing.T) {
	t.Parallel()

	// Create a fake that reports all candidates as existing.
	fake := &fakeTmux{}
	sessions := []tmux.Session{{Name: "dev"}}
	for i := 1; i <= 99; i++ {
		sessions = append(sessions, tmux.Session{Name: "dev-restored-" + func() string {
			if i < 10 {
				return "0" + string(rune('0'+i))
			}
			return string(rune('0'+i/10)) + string(rune('0'+i%10))
		}()})
	}
	fake.sessions = sessions

	// Alternatively, override SessionExists to always return true.
	allExistsFake := &allExistsTmux{}
	svc := New(newRecoveryStore(t), allExistsFake, Options{})

	_, err := svc.resolveRestoreTarget(context.Background(), "dev", ConflictRename)
	if err == nil {
		t.Fatal("expected error when all rename candidates exist")
	}
}

type allExistsTmux struct {
	fakeTmux
}

func (f *allExistsTmux) SessionExists(_ context.Context, _ string) (bool, error) {
	return true, nil
}

// ---------------------------------------------------------------------------
// Start / Stop lifecycle
// ---------------------------------------------------------------------------

func TestStartAndStop(t *testing.T) {
	t.Parallel()

	st := newRecoveryStore(t)
	fake := &fakeTmux{
		sessions: []tmux.Session{
			{Name: "dev", Attached: 1, CreatedAt: time.Now().UTC(), ActivityAt: time.Now().UTC()},
		},
		windows: map[string][]tmux.Window{
			"dev": {{Session: "dev", Index: 0, Name: "main", Active: true, Panes: 1, Layout: "abc"}},
		},
		panes: map[string][]tmux.Pane{
			"dev": {{Session: "dev", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", Active: true}},
		},
	}

	svc := New(st, fake, Options{SnapshotInterval: 50 * time.Millisecond})
	svc.bootID = func(context.Context) string { return "boot-lifecycle" }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.Start(ctx)

	// Let at least one tick run.
	time.Sleep(150 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	svc.Stop(stopCtx)

	// Verify at least one snapshot was collected.
	snapshots, err := st.ListRecoverySnapshots(context.Background(), "dev", 10)
	if err != nil {
		t.Fatalf("ListRecoverySnapshots() error = %v", err)
	}
	if len(snapshots) == 0 {
		t.Fatal("expected at least one snapshot after Start/Stop cycle")
	}
}

// ---------------------------------------------------------------------------
// Start nil receiver
// ---------------------------------------------------------------------------

func TestStartNilReceiver(t *testing.T) {
	t.Parallel()
	var svc *Service
	svc.Start(context.Background()) // should not panic
}

// ---------------------------------------------------------------------------
// Stop nil receiver
// ---------------------------------------------------------------------------

func TestStopNilReceiver(t *testing.T) {
	t.Parallel()
	var svc *Service
	svc.Stop(context.Background()) // should not panic
}

// ---------------------------------------------------------------------------
// Stop without Start
// ---------------------------------------------------------------------------

func TestStopWithoutStart(t *testing.T) {
	t.Parallel()
	svc := New(newRecoveryStore(t), &fakeTmux{}, Options{})
	svc.Stop(context.Background()) // should not panic
}

// ---------------------------------------------------------------------------
// Restore with Confirm mode sends cd but not commands
// ---------------------------------------------------------------------------

func TestRestoreConfirmModeSendsCd(t *testing.T) {
	t.Parallel()

	st := newRecoveryStore(t)
	ctx := context.Background()

	fake := &fakeTmux{
		sessions: []tmux.Session{
			{Name: "dev", Attached: 1, CreatedAt: time.Now().UTC(), ActivityAt: time.Now().UTC()},
		},
		windows: map[string][]tmux.Window{
			"dev": {{Session: "dev", Index: 0, Name: "editor", Active: true, Panes: 1, Layout: "abc"}},
		},
		panes: map[string][]tmux.Pane{
			"dev": {{Session: "dev", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", Active: true, CurrentPath: "/tmp/dev", StartCommand: "nvim", CurrentCommand: "nvim"}},
		},
	}

	svc := New(st, fake, Options{})
	svc.bootID = func(context.Context) string { return testBootID }

	if err := svc.Collect(ctx); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	snapshots, err := st.ListRecoverySnapshots(ctx, "dev", 1)
	if err != nil || len(snapshots) == 0 {
		t.Fatalf("ListRecoverySnapshots() = %v, err = %v", snapshots, err)
	}

	trackFake := &sendKeysTracker{fakeTmux: *fake}
	svc.tmux = trackFake

	job, err := svc.RestoreSnapshotAsync(ctx, snapshots[0].ID, RestoreOptions{
		Mode:           ReplayModeConfirm,
		ConflictPolicy: ConflictRename,
	})
	if err != nil {
		t.Fatalf("RestoreSnapshotAsync() error = %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		j, err := st.GetRecoveryJob(ctx, job.ID)
		if err != nil {
			t.Fatalf("GetRecoveryJob() error = %v", err)
		}
		if j.Status == store.RecoveryJobSucceeded || j.Status == store.RecoveryJobFailed {
			if j.Status == store.RecoveryJobFailed {
				t.Fatalf("restore job failed: %s", j.Error)
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	sentKeys := trackFake.keys()
	hasCd := false
	hasNvim := false
	for _, key := range sentKeys {
		if len(key) >= 3 && key[:3] == testCdPrefix {
			hasCd = true
		}
		if key == testCmdNvim {
			hasNvim = true
		}
	}
	if !hasCd {
		t.Fatal("confirm mode should send cd commands")
	}
	if hasNvim {
		t.Fatal("confirm mode should NOT send start commands")
	}
}

// ---------------------------------------------------------------------------
// Restore with Full mode sends cd and commands
// ---------------------------------------------------------------------------

func TestRestoreFullModeSendsCommands(t *testing.T) {
	t.Parallel()

	st := newRecoveryStore(t)
	ctx := context.Background()

	fake := &fakeTmux{
		sessions: []tmux.Session{
			{Name: "dev", Attached: 1, CreatedAt: time.Now().UTC(), ActivityAt: time.Now().UTC()},
		},
		windows: map[string][]tmux.Window{
			"dev": {{Session: "dev", Index: 0, Name: "editor", Active: true, Panes: 1, Layout: "abc"}},
		},
		panes: map[string][]tmux.Pane{
			"dev": {{Session: "dev", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", Active: true, CurrentPath: "/tmp/dev", StartCommand: "nvim", CurrentCommand: "nvim"}},
		},
	}

	svc := New(st, fake, Options{})
	svc.bootID = func(context.Context) string { return testBootID }

	if err := svc.Collect(ctx); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	snapshots, err := st.ListRecoverySnapshots(ctx, "dev", 1)
	if err != nil || len(snapshots) == 0 {
		t.Fatalf("ListRecoverySnapshots() = %v, err = %v", snapshots, err)
	}

	trackFake := &sendKeysTracker{fakeTmux: *fake}
	svc.tmux = trackFake

	job, err := svc.RestoreSnapshotAsync(ctx, snapshots[0].ID, RestoreOptions{
		Mode:           ReplayModeFull,
		ConflictPolicy: ConflictRename,
	})
	if err != nil {
		t.Fatalf("RestoreSnapshotAsync() error = %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		j, err := st.GetRecoveryJob(ctx, job.ID)
		if err != nil {
			t.Fatalf("GetRecoveryJob() error = %v", err)
		}
		if j.Status == store.RecoveryJobSucceeded || j.Status == store.RecoveryJobFailed {
			if j.Status == store.RecoveryJobFailed {
				t.Fatalf("restore job failed: %s", j.Error)
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	sentKeys := trackFake.keys()
	hasCd := false
	hasCmd := false
	for _, key := range sentKeys {
		if len(key) >= 3 && key[:3] == testCdPrefix {
			hasCd = true
		}
		if key == testCmdNvim {
			hasCmd = true
		}
	}
	if !hasCd {
		t.Fatal("full mode should send cd commands")
	}
	if !hasCmd {
		t.Fatal("full mode should send start commands")
	}
}

// ---------------------------------------------------------------------------
// Restore multi-window snapshot
// ---------------------------------------------------------------------------

func TestRestoreMultiWindowSnapshot(t *testing.T) {
	t.Parallel()

	st := newRecoveryStore(t)
	ctx := context.Background()

	fake := &fakeTmux{
		sessions: []tmux.Session{
			{Name: "multi", Attached: 1, CreatedAt: time.Now().UTC(), ActivityAt: time.Now().UTC()},
		},
		windows: map[string][]tmux.Window{
			"multi": {
				{Session: "multi", Index: 0, Name: "editor", Active: true, Panes: 1, Layout: "abc"},
				{Session: "multi", Index: 1, Name: "logs", Active: false, Panes: 1, Layout: "def"},
			},
		},
		panes: map[string][]tmux.Pane{
			"multi": {
				{Session: "multi", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", Active: true, CurrentPath: "/home"},
				{Session: "multi", WindowIndex: 1, PaneIndex: 0, PaneID: "%2", Active: false, CurrentPath: "/var/log"},
			},
		},
	}

	svc := New(st, fake, Options{})
	svc.bootID = func(context.Context) string { return testBootID }

	if err := svc.Collect(ctx); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	snapshots, err := st.ListRecoverySnapshots(ctx, "multi", 1)
	if err != nil || len(snapshots) == 0 {
		t.Fatalf("ListRecoverySnapshots() = %v, err = %v", snapshots, err)
	}

	trackFake := &sendKeysTracker{fakeTmux: *fake}
	svc.tmux = trackFake

	job, err := svc.RestoreSnapshotAsync(ctx, snapshots[0].ID, RestoreOptions{
		Mode:           ReplayModeConfirm,
		ConflictPolicy: ConflictRename,
	})
	if err != nil {
		t.Fatalf("RestoreSnapshotAsync() error = %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		j, err := st.GetRecoveryJob(ctx, job.ID)
		if err != nil {
			t.Fatalf("GetRecoveryJob() error = %v", err)
		}
		if j.Status == store.RecoveryJobSucceeded || j.Status == store.RecoveryJobFailed {
			if j.Status == store.RecoveryJobFailed {
				t.Fatalf("restore job failed: %s", j.Error)
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Verify that at least the first window pane got a cd command.
	// The fake tmux doesn't actually create new windows, so the
	// second window restore may not find live panes to match against.
	sentKeys := trackFake.keys()
	hasCd := false
	for _, key := range sentKeys {
		if len(key) >= 3 && key[:3] == testCdPrefix {
			hasCd = true
		}
	}
	if !hasCd {
		t.Fatal("confirm mode should send cd commands for multi-window snapshot")
	}
}

// ---------------------------------------------------------------------------
// publish nil-safe
// ---------------------------------------------------------------------------

func TestPublishNilSafe(t *testing.T) {
	t.Parallel()

	// Nil service.
	var svc *Service
	svc.publish("test", nil)

	// Nil events hub.
	svc2 := &Service{}
	svc2.publish("test", nil)
}

// ---------------------------------------------------------------------------
// raiseAlert nil-safe
// ---------------------------------------------------------------------------

func TestRaiseAlertNilSafe(t *testing.T) {
	t.Parallel()

	var svc *Service
	svc.raiseAlert(context.Background(), alerts.AlertWrite{})

	svc2 := &Service{}
	svc2.raiseAlert(context.Background(), alerts.AlertWrite{})
}

// ---------------------------------------------------------------------------
// resolveAlert nil-safe
// ---------------------------------------------------------------------------

func TestResolveAlertNilSafe(t *testing.T) {
	t.Parallel()

	var svc *Service
	svc.resolveAlert(context.Background(), "key", time.Now())

	svc2 := &Service{}
	svc2.resolveAlert(context.Background(), "key", time.Now())
}

// ---------------------------------------------------------------------------
// Collect nil guards
// ---------------------------------------------------------------------------

func TestCollectNilGuards(t *testing.T) {
	t.Parallel()

	var svc *Service
	if err := svc.Collect(context.Background()); err != nil {
		t.Fatalf("nil service Collect() error = %v", err)
	}

	svc2 := &Service{}
	if err := svc2.Collect(context.Background()); err != nil {
		t.Fatalf("nil store Collect() error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// listLiveSessions treats ErrKindServerNotRunning as empty
// ---------------------------------------------------------------------------

func TestListLiveSessionsServerNotRunning(t *testing.T) {
	t.Parallel()

	fake := &errTmux{
		listSessionsErr: &tmux.Error{Kind: tmux.ErrKindServerNotRunning, Msg: "no server"},
	}
	svc := &Service{tmux: fake}

	sessions, err := svc.listLiveSessions(context.Background())
	if err != nil {
		t.Fatalf("listLiveSessions() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions = %d, want 0", len(sessions))
	}
}

// ---------------------------------------------------------------------------
// listLiveSessions propagates other errors
// ---------------------------------------------------------------------------

func TestListLiveSessionsOtherError(t *testing.T) {
	t.Parallel()

	fake := &errTmux{
		listSessionsErr: &tmux.Error{Kind: tmux.ErrKindCommandFailed, Msg: "oops"},
	}
	svc := &Service{tmux: fake}

	_, err := svc.listLiveSessions(context.Background())
	if err == nil {
		t.Fatal("expected error for command failure")
	}
}

type errTmux struct {
	fakeTmux
	listSessionsErr error
}

func (f *errTmux) ListSessions(_ context.Context) ([]tmux.Session, error) {
	return nil, f.listSessionsErr
}

// ---------------------------------------------------------------------------
// Restore with events hub
// ---------------------------------------------------------------------------

func TestRestorePublishesEvents(t *testing.T) {
	t.Parallel()

	st := newRecoveryStore(t)
	ctx := context.Background()
	hub := events.NewHub()
	eventsCh, unsub := hub.Subscribe(32)
	defer unsub()

	fake := &fakeTmux{
		sessions: []tmux.Session{
			{Name: "dev", Attached: 1, CreatedAt: time.Now().UTC(), ActivityAt: time.Now().UTC()},
		},
		windows: map[string][]tmux.Window{
			"dev": {{Session: "dev", Index: 0, Name: "main", Active: true, Panes: 1, Layout: "abc"}},
		},
		panes: map[string][]tmux.Pane{
			"dev": {{Session: "dev", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", Active: true, CurrentPath: "/tmp"}},
		},
	}

	svc := New(st, fake, Options{EventHub: hub})
	svc.bootID = func(context.Context) string { return testBootID }

	if err := svc.Collect(ctx); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	snapshots, err := st.ListRecoverySnapshots(ctx, "dev", 1)
	if err != nil || len(snapshots) == 0 {
		t.Fatalf("ListRecoverySnapshots() error = %v", snapshots)
	}

	job, err := svc.RestoreSnapshotAsync(ctx, snapshots[0].ID, RestoreOptions{
		Mode:           ReplayModeSafe,
		ConflictPolicy: ConflictRename,
	})
	if err != nil {
		t.Fatalf("RestoreSnapshotAsync() error = %v", err)
	}

	// Wait for job to finish.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		j, err := st.GetRecoveryJob(ctx, job.ID)
		if err != nil {
			t.Fatalf("GetRecoveryJob() error = %v", err)
		}
		if j.Status == store.RecoveryJobSucceeded || j.Status == store.RecoveryJobFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Drain events and check for recovery-related types.
	gotRecoveryJob := false
	gotRecoveryOverview := false
	drainTimeout := time.After(2 * time.Second)
	for {
		select {
		case evt := <-eventsCh:
			if evt.Type == events.TypeRecoveryJob {
				gotRecoveryJob = true
			}
			if evt.Type == events.TypeRecoveryOverview {
				gotRecoveryOverview = true
			}
		case <-drainTimeout:
			goto done
		}
	}
done:
	if !gotRecoveryJob {
		t.Fatal("expected TypeRecoveryJob event")
	}
	if !gotRecoveryOverview {
		t.Fatal("expected TypeRecoveryOverview event")
	}
}

// ---------------------------------------------------------------------------
// resolveRestoreTarget — invalid conflict policy
// ---------------------------------------------------------------------------

func TestResolveRestoreTargetInvalidPolicy(t *testing.T) {
	t.Parallel()

	fake := &fakeTmux{
		sessions: []tmux.Session{{Name: "dev"}},
	}
	svc := New(newRecoveryStore(t), fake, Options{})

	_, err := svc.resolveRestoreTarget(context.Background(), "dev", ConflictPolicy("invalid"))
	if err == nil {
		t.Fatal("expected error for invalid conflict policy")
	}
}

// ---------------------------------------------------------------------------
// ensurePaneCount — wanted <= 1 is a no-op
// ---------------------------------------------------------------------------

func TestEnsurePaneCountSinglePane(t *testing.T) {
	t.Parallel()

	fake := &fakeTmux{}
	svc := New(newRecoveryStore(t), fake, Options{})

	err := svc.ensurePaneCount(context.Background(), "test", 0, 1, nil)
	if err != nil {
		t.Fatalf("ensurePaneCount(1) error = %v", err)
	}
	err = svc.ensurePaneCount(context.Background(), "test", 0, 0, nil)
	if err != nil {
		t.Fatalf("ensurePaneCount(0) error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// ensurePaneCount — splits panes to reach target count
// ---------------------------------------------------------------------------

func TestEnsurePaneCountSplits(t *testing.T) {
	t.Parallel()

	// Use a fake that tracks how many panes exist after splits.
	splitFake := &splitTrackingTmux{
		fakeTmux: fakeTmux{
			panes: map[string][]tmux.Pane{
				"test": {
					{Session: "test", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", Active: true},
				},
			},
		},
	}
	svc := New(newRecoveryStore(t), splitFake, Options{})

	snapPanes := []PaneSnapshot{
		{PaneIndex: 0, CurrentPath: "/home"},
		{PaneIndex: 1, CurrentPath: "/tmp"},
	}
	err := svc.ensurePaneCount(context.Background(), "test", 0, 2, snapPanes)
	if err != nil {
		t.Fatalf("ensurePaneCount(2) error = %v", err)
	}
	if splitFake.splitCount == 0 {
		t.Fatal("expected at least one split")
	}
}

type splitTrackingTmux struct {
	fakeTmux
	splitCount int
}

func (f *splitTrackingTmux) ListPanes(_ context.Context, session string) ([]tmux.Pane, error) {
	return append([]tmux.Pane{}, f.panes[session]...), nil
}

func (f *splitTrackingTmux) SplitPaneIn(_ context.Context, paneID, direction, cwd string) error {
	f.splitCount++
	// Simulate a new pane being added.
	for sessionName, panes := range f.panes {
		newIdx := len(panes)
		f.panes[sessionName] = append(panes, tmux.Pane{
			Session:     sessionName,
			WindowIndex: 0,
			PaneIndex:   newIdx,
			PaneID:      "%" + string(rune('0'+newIdx+1)),
			Active:      false,
		})
		break
	}
	return nil
}

// ---------------------------------------------------------------------------
// firstPaneCWDForWindow — no panes returns empty
// ---------------------------------------------------------------------------

func TestFirstPaneCWDForWindowEmpty(t *testing.T) {
	t.Parallel()

	got := firstPaneCWDForWindow(nil, 0)
	if got != "" {
		t.Fatalf("firstPaneCWDForWindow(nil, 0) = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// firstPaneCWDForWindow — returns first pane CWD sorted by index
// ---------------------------------------------------------------------------

func TestFirstPaneCWDForWindowSorted(t *testing.T) {
	t.Parallel()

	panes := []PaneSnapshot{
		{WindowIndex: 0, PaneIndex: 2, CurrentPath: "/last"},
		{WindowIndex: 0, PaneIndex: 0, CurrentPath: "/first"},
		{WindowIndex: 0, PaneIndex: 1, CurrentPath: "/middle"},
	}
	got := firstPaneCWDForWindow(panes, 0)
	if got != "/first" {
		t.Fatalf("firstPaneCWDForWindow() = %q, want /first", got)
	}
}

// ---------------------------------------------------------------------------
// panesForWindow
// ---------------------------------------------------------------------------

func TestPanesForWindow(t *testing.T) {
	t.Parallel()

	panes := []PaneSnapshot{
		{WindowIndex: 0, PaneIndex: 0},
		{WindowIndex: 1, PaneIndex: 0},
		{WindowIndex: 0, PaneIndex: 1},
		{WindowIndex: 2, PaneIndex: 0},
	}
	got := panesForWindow(panes, 0)
	if len(got) != 2 {
		t.Fatalf("panesForWindow(0) len = %d, want 2", len(got))
	}
	got = panesForWindow(panes, 3)
	if len(got) != 0 {
		t.Fatalf("panesForWindow(3) len = %d, want 0", len(got))
	}
}

// ---------------------------------------------------------------------------
// filterPanesByWindow
// ---------------------------------------------------------------------------

func TestFilterPanesByWindow(t *testing.T) {
	t.Parallel()

	all := []tmux.Pane{
		{WindowIndex: 0, PaneIndex: 0, PaneID: "%1"},
		{WindowIndex: 1, PaneIndex: 0, PaneID: "%2"},
		{WindowIndex: 0, PaneIndex: 1, PaneID: "%3"},
	}
	got := filterPanesByWindow(all, 0)
	if len(got) != 2 {
		t.Fatalf("filterPanesByWindow(0) len = %d, want 2", len(got))
	}
	got = filterPanesByWindow(all, 5)
	if len(got) != 0 {
		t.Fatalf("filterPanesByWindow(5) len = %d, want 0", len(got))
	}
}

// ---------------------------------------------------------------------------
// snapshotHash — deterministic and unique
// ---------------------------------------------------------------------------

func TestSnapshotHash(t *testing.T) {
	t.Parallel()

	snapA := SessionSnapshot{
		SessionName:  "dev",
		ActiveWindow: 0,
		Windows:      []WindowSnapshot{{Index: 0, Name: "main", Panes: 1}},
		Panes:        []PaneSnapshot{{WindowIndex: 0, PaneIndex: 0, CurrentPath: "/tmp"}},
	}
	snapB := SessionSnapshot{
		SessionName:  "dev",
		ActiveWindow: 0,
		Windows:      []WindowSnapshot{{Index: 0, Name: "main", Panes: 1}},
		Panes:        []PaneSnapshot{{WindowIndex: 0, PaneIndex: 0, CurrentPath: "/var"}},
	}

	hashA, err := snapshotHash(snapA)
	if err != nil {
		t.Fatalf("snapshotHash(A) error = %v", err)
	}
	hashA2, err := snapshotHash(snapA)
	if err != nil {
		t.Fatalf("snapshotHash(A2) error = %v", err)
	}
	hashB, err := snapshotHash(snapB)
	if err != nil {
		t.Fatalf("snapshotHash(B) error = %v", err)
	}

	if hashA != hashA2 {
		t.Fatalf("snapshotHash not deterministic: %q != %q", hashA, hashA2)
	}
	if hashA == hashB {
		t.Fatalf("snapshotHash should differ for different snapshots: %q == %q", hashA, hashB)
	}
}

// ---------------------------------------------------------------------------
// resolveAlert — non-ErrNoRows error is tolerated
// ---------------------------------------------------------------------------

func TestResolveAlertNonNoRowsError(t *testing.T) {
	t.Parallel()

	alertRepo := &fakeAlertRepo{
		resolveErr: errors.New("db error"),
	}
	svc := &Service{options: Options{AlertRepo: alertRepo}}
	// Should not panic, just log a warning.
	svc.resolveAlert(context.Background(), "test-key", time.Now())

	alertRepo.mu.Lock()
	defer alertRepo.mu.Unlock()
	if len(alertRepo.resolved) != 1 {
		t.Fatalf("expected 1 resolved call, got %d", len(alertRepo.resolved))
	}
}

// ---------------------------------------------------------------------------
// captureLiveSessions — skips empty session names
// ---------------------------------------------------------------------------

func TestCaptureLiveSessionsSkipsEmpty(t *testing.T) {
	t.Parallel()

	st := newRecoveryStore(t)
	fake := &fakeTmux{
		windows: map[string][]tmux.Window{},
		panes:   map[string][]tmux.Pane{},
	}
	svc := New(st, fake, Options{})
	svc.bootID = func(context.Context) string { return "boot" }

	sessions := []tmux.Session{
		{Name: ""},
		{Name: "  "},
	}
	liveNames, liveList, changedCount := svc.captureLiveSessions(context.Background(), sessions, "boot", time.Now().UTC())
	if len(liveNames) != 0 {
		t.Fatalf("liveNames len = %d, want 0", len(liveNames))
	}
	if len(liveList) != 0 {
		t.Fatalf("liveList len = %d, want 0", len(liveList))
	}
	if changedCount != 0 {
		t.Fatalf("changedCount = %d, want 0", changedCount)
	}
}

// ---------------------------------------------------------------------------
// bootStateChanged — empty bootID or empty lastBootID
// ---------------------------------------------------------------------------

func TestBootStateChangedEmptyValues(t *testing.T) {
	t.Parallel()

	st := newRecoveryStore(t)
	svc := &Service{store: st}

	// No previous boot ID stored — should return false.
	changed, err := svc.bootStateChanged(context.Background(), "boot-new")
	if err != nil {
		t.Fatalf("bootStateChanged error = %v", err)
	}
	if changed {
		t.Fatal("expected false when no previous boot ID")
	}

	// Store a boot ID, then test with same value.
	if err := st.SetRuntimeValue(context.Background(), runtimeBootIDKey, "boot-new"); err != nil {
		t.Fatalf("SetRuntimeValue error = %v", err)
	}
	changed, err = svc.bootStateChanged(context.Background(), "boot-new")
	if err != nil {
		t.Fatalf("bootStateChanged error = %v", err)
	}
	if changed {
		t.Fatal("expected false when boot ID matches")
	}

	// Now with a different value.
	changed, err = svc.bootStateChanged(context.Background(), "boot-different")
	if err != nil {
		t.Fatalf("bootStateChanged error = %v", err)
	}
	if !changed {
		t.Fatal("expected true when boot ID differs")
	}
}

// ---------------------------------------------------------------------------
// Overview — boot change timestamp is populated
// ---------------------------------------------------------------------------

func TestOverviewWithBootChange(t *testing.T) {
	t.Parallel()

	st := newRecoveryStore(t)
	ctx := context.Background()

	fake := &fakeTmux{
		sessions: []tmux.Session{
			{Name: "dev", Attached: 1, CreatedAt: time.Now().UTC(), ActivityAt: time.Now().UTC()},
		},
		windows: map[string][]tmux.Window{
			"dev": {{Session: "dev", Index: 0, Name: "main", Active: true, Panes: 1, Layout: "abc"}},
		},
		panes: map[string][]tmux.Pane{
			"dev": {{Session: "dev", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", Active: true}},
		},
	}

	bootID := "boot-ov-1"
	svc := New(st, fake, Options{})
	svc.bootID = func(context.Context) string { return bootID }

	if err := svc.Collect(ctx); err != nil {
		t.Fatalf("first Collect error = %v", err)
	}

	// Boot change.
	bootID = "boot-ov-2"
	fake.sessions = nil
	fake.windows = map[string][]tmux.Window{}
	fake.panes = map[string][]tmux.Pane{}
	if err := svc.Collect(ctx); err != nil {
		t.Fatalf("second Collect error = %v", err)
	}

	overview, err := svc.Overview(ctx)
	if err != nil {
		t.Fatalf("Overview error = %v", err)
	}
	if overview.LastBootChange.IsZero() {
		t.Fatal("LastBootChange should be non-zero after boot change")
	}
	if len(overview.KilledSessions) == 0 {
		t.Fatal("expected at least one killed session in overview")
	}
}

// ---------------------------------------------------------------------------
// publishCollectEvents — covers all branches
// ---------------------------------------------------------------------------

func TestPublishCollectEventsNoBroadcast(t *testing.T) {
	t.Parallel()

	hub := events.NewHub()
	eventsCh, unsub := hub.Subscribe(8)
	defer unsub()
	svc := &Service{events: hub}

	// No changes — no events should be published.
	svc.publishCollectEvents(false, 0, 0, 0, false)

	select {
	case evt := <-eventsCh:
		t.Fatalf("unexpected event: %+v", evt)
	default:
	}
}

func TestPublishCollectEventsLiveSetChanged(t *testing.T) {
	t.Parallel()

	hub := events.NewHub()
	eventsCh, unsub := hub.Subscribe(8)
	defer unsub()
	svc := &Service{events: hub}

	// Live set changed + changedCount > 0.
	svc.publishCollectEvents(true, 2, 1, 0, false)

	gotTypes := map[string]bool{}
	for len(gotTypes) < 2 {
		select {
		case evt := <-eventsCh:
			gotTypes[evt.Type] = true
		default:
			if len(gotTypes) >= 2 {
				break
			}
			continue
		}
	}
	if !gotTypes[events.TypeTmuxSessions] {
		t.Fatal("expected TypeTmuxSessions event for live set change")
	}
	if !gotTypes[events.TypeRecoveryOverview] {
		t.Fatal("expected TypeRecoveryOverview event for changed sessions")
	}
}

// ---------------------------------------------------------------------------
// restoreSession — empty windows returns error
// ---------------------------------------------------------------------------

func TestRestoreSessionEmptyWindows(t *testing.T) {
	t.Parallel()

	fake := &fakeTmux{}
	svc := New(newRecoveryStore(t), fake, Options{})

	err := svc.restoreSession(context.Background(), SessionSnapshot{}, ReplayModeSafe, "test", func(string) {})
	if err == nil {
		t.Fatal("expected error for snapshot with no windows")
	}
}
