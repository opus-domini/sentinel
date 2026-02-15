package ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/service"
)

const (
	ServiceNameSentinel = "sentinel"
	ServiceNameUpdater  = "sentinel-updater"

	ActionStart   = "start"
	ActionStop    = "stop"
	ActionRestart = "restart"

	scopeUser   = "user"
	scopeSystem = "system"

	managerSystemd = "systemd"
	managerLaunchd = "launchd"

	sentinelSystemdUnit = "sentinel"
	updaterSystemdUnit  = "sentinel-updater.timer"

	sentinelLaunchdLabel = "io.opusdomini.sentinel"
	updaterLaunchdLabel  = "io.opusdomini.sentinel.updater"
)

var (
	ErrServiceNotFound = errors.New("ops service not found")
	ErrInvalidAction   = errors.New("ops invalid action")
)

type commandRunner func(ctx context.Context, name string, args ...string) (string, error)

type ServiceStatus struct {
	Name         string `json:"name"`
	DisplayName  string `json:"displayName"`
	Manager      string `json:"manager"`
	Scope        string `json:"scope"`
	Unit         string `json:"unit"`
	Exists       bool   `json:"exists"`
	EnabledState string `json:"enabledState"`
	ActiveState  string `json:"activeState"`
	LastRunState string `json:"lastRunState,omitempty"`
	UpdatedAt    string `json:"updatedAt"`
}

type HostOverview struct {
	Hostname  string `json:"hostname"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	CPUs      int    `json:"cpus"`
	GoVersion string `json:"goVersion"`
}

type SentinelOverview struct {
	PID       int   `json:"pid"`
	UptimeSec int64 `json:"uptimeSec"`
}

type ServicesSummary struct {
	Total  int `json:"total"`
	Active int `json:"active"`
	Failed int `json:"failed"`
}

type Overview struct {
	Host      HostOverview     `json:"host"`
	Sentinel  SentinelOverview `json:"sentinel"`
	Services  ServicesSummary  `json:"services"`
	UpdatedAt string           `json:"updatedAt"`
}

type Manager struct {
	startedAt time.Time
	nowFn     func() time.Time
	hostname  func() (string, error)
	uidFn     func() int
	goos      string

	userStatusFn       func() (service.UserServiceStatus, error)
	autoUpdateStatusFn func(scope string) (service.UserAutoUpdateServiceStatus, error)
	userServicePathFn  func() (string, error)
	autoServicePathFn  func(scope string) (string, error)
	commandRunner      commandRunner
}

func NewManager(startedAt time.Time) *Manager {
	now := time.Now().UTC()
	if startedAt.IsZero() {
		startedAt = now
	}
	return &Manager{
		startedAt:          startedAt,
		nowFn:              time.Now,
		hostname:           os.Hostname,
		uidFn:              os.Getuid,
		goos:               runtime.GOOS,
		userStatusFn:       service.UserStatus,
		autoUpdateStatusFn: service.UserAutoUpdateStatusForScope,
		userServicePathFn:  service.UserServicePath,
		autoServicePathFn:  service.UserAutoUpdateServicePathForScope,
		commandRunner:      runCommand,
	}
}

func (m *Manager) Overview(ctx context.Context) (Overview, error) {
	services, err := m.ListServices(ctx)
	if err != nil {
		return Overview{}, err
	}

	now := m.nowFn().UTC()
	hostname, _ := m.hostname()
	uptime := now.Sub(m.startedAt)
	if uptime < 0 {
		uptime = 0
	}

	out := Overview{
		Host: HostOverview{
			Hostname:  strings.TrimSpace(hostname),
			OS:        m.goos,
			Arch:      runtime.GOARCH,
			CPUs:      runtime.NumCPU(),
			GoVersion: runtime.Version(),
		},
		Sentinel: SentinelOverview{
			PID:       os.Getpid(),
			UptimeSec: int64(uptime / time.Second),
		},
		UpdatedAt: now.Format(time.RFC3339),
	}

	out.Services.Total = len(services)
	for _, item := range services {
		switch strings.ToLower(strings.TrimSpace(item.ActiveState)) {
		case "active", "running":
			out.Services.Active++
		case "failed":
			out.Services.Failed++
		}
	}

	return out, nil
}

func (m *Manager) ListServices(ctx context.Context) ([]ServiceStatus, error) {
	_ = ctx
	baseStatus, err := m.userStatusFn()
	if err != nil {
		return nil, err
	}

	scope := detectScope(baseStatus.ServicePath, m.uidFn)
	manager := detectManager(m.goos)

	updaterStatus, err := m.autoUpdateStatusFn(scope)
	if err != nil {
		return nil, err
	}

	now := m.nowFn().UTC().Format(time.RFC3339)
	return []ServiceStatus{
		{
			Name:         ServiceNameSentinel,
			DisplayName:  "Sentinel service",
			Manager:      manager,
			Scope:        scope,
			Unit:         unitForService(manager, ServiceNameSentinel),
			Exists:       baseStatus.UnitFileExists,
			EnabledState: normalizeState(baseStatus.EnabledState),
			ActiveState:  normalizeState(baseStatus.ActiveState),
			UpdatedAt:    now,
		},
		{
			Name:         ServiceNameUpdater,
			DisplayName:  "Autoupdate timer",
			Manager:      manager,
			Scope:        scope,
			Unit:         unitForService(manager, ServiceNameUpdater),
			Exists:       updaterStatus.ServiceUnitExists || updaterStatus.TimerUnitExists,
			EnabledState: normalizeState(updaterStatus.TimerEnabledState),
			ActiveState:  normalizeState(updaterStatus.TimerActiveState),
			LastRunState: normalizeState(updaterStatus.LastRunState),
			UpdatedAt:    now,
		},
	}, nil
}

func (m *Manager) Act(ctx context.Context, name, action string) (ServiceStatus, error) {
	serviceName, ok := normalizeServiceName(name)
	if !ok {
		return ServiceStatus{}, ErrServiceNotFound
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if !isValidAction(action) {
		return ServiceStatus{}, ErrInvalidAction
	}

	services, err := m.ListServices(ctx)
	if err != nil {
		return ServiceStatus{}, err
	}
	target, ok := findServiceStatus(services, serviceName)
	if !ok {
		return ServiceStatus{}, ErrServiceNotFound
	}

	switch target.Manager {
	case managerSystemd:
		if err := m.actSystemd(ctx, target.Scope, serviceName, action); err != nil {
			return ServiceStatus{}, err
		}
	case managerLaunchd:
		if err := m.actLaunchd(ctx, target.Scope, serviceName, action); err != nil {
			return ServiceStatus{}, err
		}
	default:
		return ServiceStatus{}, fmt.Errorf("unsupported service manager: %s", target.Manager)
	}

	updated, err := m.ListServices(ctx)
	if err != nil {
		return ServiceStatus{}, err
	}
	if next, found := findServiceStatus(updated, serviceName); found {
		return next, nil
	}
	return ServiceStatus{}, ErrServiceNotFound
}

func (m *Manager) actSystemd(ctx context.Context, scope, serviceName, action string) error {
	args := make([]string, 0, 4)
	if strings.EqualFold(scope, scopeUser) {
		args = append(args, "--user")
	}
	args = append(args, action, unitForService(managerSystemd, serviceName))
	_, err := m.commandRunner(ctx, "systemctl", args...)
	if err != nil {
		return fmt.Errorf("systemd action failed: %w", err)
	}
	return nil
}

func (m *Manager) actLaunchd(ctx context.Context, scope, serviceName, action string) error {
	label := unitForService(managerLaunchd, serviceName)
	target := launchdTarget(scope, m.uidFn, label)
	domain := launchdDomain(scope, m.uidFn)

	switch action {
	case ActionStop:
		_, err := m.commandRunner(ctx, "launchctl", "bootout", target)
		if err != nil && !isLaunchdMissingJobError(err) {
			return fmt.Errorf("launchd stop failed: %w", err)
		}
		return nil
	case ActionStart, ActionRestart:
		if loaded, _ := m.isLaunchdLoaded(ctx, target); !loaded {
			plistPath, pathErr := m.plistPathForService(serviceName, scope)
			if pathErr != nil {
				return pathErr
			}
			if strings.TrimSpace(plistPath) == "" {
				return fmt.Errorf("launchd plist path unavailable for %s", serviceName)
			}
			if _, err := m.commandRunner(ctx, "launchctl", "bootstrap", domain, plistPath); err != nil && !isLaunchdAlreadyLoadedError(err) {
				return fmt.Errorf("launchd bootstrap failed: %w", err)
			}
		}
		_, err := m.commandRunner(ctx, "launchctl", "kickstart", "-k", target)
		if err != nil {
			return fmt.Errorf("launchd %s failed: %w", action, err)
		}
		return nil
	default:
		return ErrInvalidAction
	}
}

func (m *Manager) isLaunchdLoaded(ctx context.Context, target string) (bool, error) {
	_, err := m.commandRunner(ctx, "launchctl", "print", target)
	if err != nil {
		if isLaunchdMissingJobError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (m *Manager) plistPathForService(serviceName, scope string) (string, error) {
	switch serviceName {
	case ServiceNameSentinel:
		return m.userServicePathFn()
	case ServiceNameUpdater:
		return m.autoServicePathFn(scope)
	default:
		return "", ErrServiceNotFound
	}
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		if msg == "" {
			return "", fmt.Errorf("%s %s failed: %w", name, strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("%s %s failed: %s", name, strings.Join(args, " "), msg)
	}
	return msg, nil
}

func detectManager(goos string) string {
	if strings.EqualFold(goos, "darwin") {
		return managerLaunchd
	}
	return managerSystemd
}

func detectScope(path string, uidFn func() int) string {
	cleaned := strings.ToLower(strings.TrimSpace(path))
	switch {
	case strings.Contains(cleaned, "/etc/systemd/system/"),
		strings.Contains(cleaned, "/library/launchdaemons/"):
		return scopeSystem
	case strings.Contains(cleaned, "/.config/systemd/user/"),
		strings.Contains(cleaned, "/library/launchagents/"):
		return scopeUser
	default:
		if uidFn != nil && uidFn() == 0 {
			return scopeSystem
		}
		return scopeUser
	}
}

func normalizeState(raw string) string {
	state := strings.TrimSpace(raw)
	if state == "" {
		return "-"
	}
	return state
}

func unitForService(manager, serviceName string) string {
	if strings.EqualFold(manager, managerLaunchd) {
		if serviceName == ServiceNameUpdater {
			return updaterLaunchdLabel
		}
		return sentinelLaunchdLabel
	}
	if serviceName == ServiceNameUpdater {
		return updaterSystemdUnit
	}
	return sentinelSystemdUnit
}

func normalizeServiceName(raw string) (string, bool) {
	name := strings.ToLower(strings.TrimSpace(raw))
	switch name {
	case ServiceNameSentinel, "sentinel.service":
		return ServiceNameSentinel, true
	case ServiceNameUpdater, "sentinel-updater.service", "sentinel-updater.timer", "updater":
		return ServiceNameUpdater, true
	default:
		return "", false
	}
}

func findServiceStatus(list []ServiceStatus, name string) (ServiceStatus, bool) {
	for _, item := range list {
		if item.Name == name {
			return item, true
		}
	}
	return ServiceStatus{}, false
}

func isValidAction(action string) bool {
	switch action {
	case ActionStart, ActionStop, ActionRestart:
		return true
	default:
		return false
	}
}

func launchdDomain(scope string, uidFn func() int) string {
	if strings.EqualFold(scope, scopeSystem) {
		return scopeSystem
	}
	uid := 0
	if uidFn != nil {
		uid = uidFn()
	}
	return fmt.Sprintf("gui/%d", uid)
}

func launchdTarget(scope string, uidFn func() int, label string) string {
	return launchdDomain(scope, uidFn) + "/" + label
}

func isLaunchdMissingJobError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "could not find service") ||
		strings.Contains(msg, "no such process") ||
		strings.Contains(msg, "service is disabled")
}

func isLaunchdAlreadyLoadedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "service already loaded")
}
