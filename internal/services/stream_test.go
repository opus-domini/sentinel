package services

import (
	"errors"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/service"
)

func TestStreamLogs_LaunchdUnsupported(t *testing.T) {
	t.Parallel()

	m := &Manager{
		nowFn: time.Now,
		goos:  "darwin",
		uidFn: func() int { return 1000 },
		userStatusFn: func() (service.UserServiceStatus, error) {
			return service.UserServiceStatus{
				ServicePath:    "/Users/dev/Library/LaunchAgents/io.opusdomini.sentinel.plist",
				UnitFileExists: true,
				EnabledState:   "loaded",
				ActiveState:    "active",
			}, nil
		},
		autoUpdateStatusFn: func(string) (service.UserAutoUpdateServiceStatus, error) {
			return service.UserAutoUpdateServiceStatus{}, nil
		},
	}

	_, err := m.StreamLogs(t.Context(), "sentinel")
	if err == nil {
		t.Fatal("expected error for launchd streaming")
	}
	if !errors.Is(err, ErrStreamingUnsupported) {
		t.Errorf("expected ErrStreamingUnsupported, got: %v", err)
	}
}

func TestStreamLogsByUnit_LaunchdUnsupported(t *testing.T) {
	t.Parallel()

	m := &Manager{
		goos: "darwin",
	}

	_, err := m.StreamLogsByUnit(t.Context(), "com.example.app", "system", "launchd")
	if err == nil {
		t.Fatal("expected error for launchd streaming")
	}
	if !errors.Is(err, ErrStreamingUnsupported) {
		t.Errorf("expected ErrStreamingUnsupported, got: %v", err)
	}
}

func TestStreamLogs_ServiceNotFound(t *testing.T) {
	t.Parallel()

	m := &Manager{
		goos: "linux",
	}

	_, err := m.StreamLogs(t.Context(), "")
	if err == nil {
		t.Fatal("expected error for empty service name")
	}
	if !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("expected ErrServiceNotFound, got: %v", err)
	}
}

func TestStreamLogsByUnit_UnsupportedManager(t *testing.T) {
	t.Parallel()

	m := &Manager{}

	_, err := m.StreamLogsByUnit(t.Context(), "unit", "user", "unknown")
	if err == nil {
		t.Fatal("expected error for unknown manager")
	}
	if !errors.Is(err, ErrStreamingUnsupported) {
		t.Errorf("expected ErrStreamingUnsupported, got: %v", err)
	}
}
