package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/service"
	"github.com/opus-domini/sentinel/internal/store"
)

// customServicesRepo defines the store operations consumed by Manager.
type customServicesRepo interface {
	ListCustomServices(ctx context.Context) ([]store.CustomService, error)
}

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

	stateActive  = "active"
	stateRunning = "running"
	stateUnknown = "unknown"
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

type ServiceInspect struct {
	Service    ServiceStatus     `json:"service"`
	Summary    string            `json:"summary"`
	Properties map[string]string `json:"properties,omitempty"`
	Output     string            `json:"output,omitempty"`
	CheckedAt  string            `json:"checkedAt"`
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
	startedAt      time.Time
	nowFn          func() time.Time
	hostname       func() (string, error)
	uidFn          func() int
	goos           string
	customServices customServicesRepo

	userStatusFn       func() (service.UserServiceStatus, error)
	autoUpdateStatusFn func(scope string) (service.UserAutoUpdateServiceStatus, error)
	installAutoUpdate  func(opts service.InstallUserAutoUpdateOptions) error
	userServicePathFn  func() (string, error)
	autoServicePathFn  func(scope string) (string, error)
	commandRunner      commandRunner
}

func NewManager(startedAt time.Time, csRepo customServicesRepo) *Manager {
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
		customServices:     csRepo,
		userStatusFn:       service.UserStatus,
		autoUpdateStatusFn: service.UserAutoUpdateStatusForScope,
		installAutoUpdate:  service.InstallUserAutoUpdate,
		userServicePathFn:  service.UserServicePath,
		autoServicePathFn:  service.UserAutoUpdateServicePathForScope,
		commandRunner:      runCommand,
	}
}

func (m *Manager) Metrics(ctx context.Context) HostMetrics {
	return CollectMetrics(ctx, "/")
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
		case stateActive, stateRunning:
			out.Services.Active++
		case "failed":
			out.Services.Failed++
		}
	}

	return out, nil
}

func (m *Manager) ListServices(ctx context.Context) ([]ServiceStatus, error) {
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
	services := []ServiceStatus{
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
	}

	// Merge custom services from store.
	if m.customServices != nil {
		custom, err := m.customServices.ListCustomServices(ctx)
		if err == nil {
			for _, cs := range custom {
				svc := ServiceStatus{
					Name:        cs.Name,
					DisplayName: cs.DisplayName,
					Manager:     cs.Manager,
					Unit:        cs.Unit,
					Scope:       cs.Scope,
					UpdatedAt:   now,
				}
				m.probeCustomService(ctx, &svc)
				services = append(services, svc)
			}
		}
	}

	return services, nil
}

func (m *Manager) probeCustomService(ctx context.Context, svc *ServiceStatus) {
	switch svc.Manager {
	case managerSystemd:
		args := make([]string, 0, 6)
		if strings.EqualFold(svc.Scope, scopeUser) {
			args = append(args, "--user")
		}
		args = append(args, "show", svc.Unit, "--no-pager",
			"--property=UnitFileState,ActiveState,LoadState")
		out, err := m.commandRunner(ctx, "systemctl", args...)
		if err != nil {
			svc.Exists = false
			svc.ActiveState = stateUnknown
			svc.EnabledState = stateUnknown
			return
		}
		props := parseSystemdShow(out)
		svc.Exists = props["LoadState"] != "not-found"
		svc.ActiveState = normalizeState(props["ActiveState"])
		svc.EnabledState = normalizeState(props["UnitFileState"])
	case managerLaunchd:
		label := svc.Unit
		target := launchdTarget(svc.Scope, m.uidFn, label)
		_, err := m.commandRunner(ctx, "launchctl", "print", target)
		if err != nil {
			svc.Exists = false
			svc.ActiveState = "inactive"
			svc.EnabledState = "-"
			return
		}
		svc.Exists = true
		svc.ActiveState = stateRunning
		svc.EnabledState = "enabled"
	default:
		svc.Exists = false
		svc.ActiveState = stateUnknown
		svc.EnabledState = stateUnknown
	}
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
	if err := m.ensureServiceActionReady(target, action); err != nil {
		return ServiceStatus{}, err
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

func (m *Manager) Inspect(ctx context.Context, name string) (ServiceInspect, error) {
	serviceName, ok := normalizeServiceName(name)
	if !ok {
		return ServiceInspect{}, ErrServiceNotFound
	}

	services, err := m.ListServices(ctx)
	if err != nil {
		return ServiceInspect{}, err
	}
	target, ok := findServiceStatus(services, serviceName)
	if !ok {
		return ServiceInspect{}, ErrServiceNotFound
	}

	inspect := ServiceInspect{
		Service:   target,
		Summary:   fmt.Sprintf("enabled=%s active=%s", target.EnabledState, target.ActiveState),
		CheckedAt: m.nowFn().UTC().Format(time.RFC3339),
	}

	switch target.Manager {
	case managerSystemd:
		props, output, inspectErr := m.inspectSystemd(ctx, target)
		if inspectErr != nil {
			return ServiceInspect{}, inspectErr
		}
		inspect.Properties = props
		inspect.Output = output
		if summary := strings.TrimSpace(buildInspectSummary(props)); summary != "" {
			inspect.Summary = summary
		}
	case managerLaunchd:
		output, inspectErr := m.inspectLaunchd(ctx, target)
		if inspectErr != nil {
			return ServiceInspect{}, inspectErr
		}
		inspect.Output = output
	default:
		return ServiceInspect{}, fmt.Errorf("unsupported service manager: %s", target.Manager)
	}

	return inspect, nil
}

func (m *Manager) actSystemd(ctx context.Context, scope, serviceName, action string) error {
	args := make([]string, 0, 4)
	if strings.EqualFold(scope, scopeUser) {
		args = append(args, "--user")
	}
	args = append(args, action, unitForService(managerSystemd, serviceName))
	_, err := m.commandRunner(ctx, "systemctl", args...)
	if err != nil {
		if serviceName == ServiceNameUpdater &&
			(action == ActionStart || action == ActionRestart) &&
			isSystemdUnitNotFoundError(err) {
			if bootstrapErr := m.bootstrapUpdater(scope); bootstrapErr != nil {
				return bootstrapErr
			}
			if _, retryErr := m.commandRunner(ctx, "systemctl", args...); retryErr != nil {
				return fmt.Errorf("systemd action failed after autoupdate bootstrap: %w", retryErr)
			}
			return nil
		}
		return fmt.Errorf("systemd action failed: %w", err)
	}
	return nil
}

func (m *Manager) inspectSystemd(ctx context.Context, target ServiceStatus) (map[string]string, string, error) {
	args := make([]string, 0, 12)
	if strings.EqualFold(target.Scope, scopeUser) {
		args = append(args, "--user")
	}
	args = append(args,
		"show",
		target.Unit,
		"--no-pager",
		"--property=Id,Description,LoadState,UnitFileState,ActiveState,SubState,FragmentPath,ExecMainPID",
	)
	out, err := m.commandRunner(ctx, "systemctl", args...)
	if err != nil {
		return nil, "", fmt.Errorf("systemd inspect failed: %w", err)
	}
	props := parseSystemdShow(out)
	return props, out, nil
}

func (m *Manager) inspectLaunchd(ctx context.Context, target ServiceStatus) (string, error) {
	label := unitForService(managerLaunchd, target.Name)
	out, err := m.commandRunner(ctx, "launchctl", "print", launchdTarget(target.Scope, m.uidFn, label))
	if err != nil {
		return "", fmt.Errorf("launchd inspect failed: %w", err)
	}
	return out, nil
}

func (m *Manager) ensureServiceActionReady(target ServiceStatus, action string) error {
	if target.Name != ServiceNameUpdater {
		return nil
	}
	if action != ActionStart && action != ActionRestart {
		return nil
	}
	if target.Exists {
		return nil
	}
	return m.bootstrapUpdater(target.Scope)
}

func (m *Manager) bootstrapUpdater(scope string) error {
	installFn := m.installAutoUpdate
	if installFn == nil {
		installFn = service.InstallUserAutoUpdate
	}
	if err := installFn(service.InstallUserAutoUpdateOptions{
		Enable:          true,
		Start:           true,
		ServiceUnit:     ServiceNameSentinel,
		SystemdScope:    scope,
		OnCalendar:      "daily",
		RandomizedDelay: time.Hour,
	}); err != nil {
		return fmt.Errorf("autoupdate bootstrap failed: %w", err)
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
	name := strings.TrimSpace(raw)
	lower := strings.ToLower(name)
	switch lower {
	case ServiceNameSentinel, "sentinel.service":
		return ServiceNameSentinel, true
	case ServiceNameUpdater, "sentinel-updater.service", "sentinel-updater.timer", "updater":
		return ServiceNameUpdater, true
	default:
		// Accept custom service names (non-empty).
		if name != "" {
			return name, true
		}
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

func isSystemdUnitNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "unit sentinel-updater.timer not found") ||
		strings.Contains(msg, "could not be found") ||
		strings.Contains(msg, "no such file or directory")
}

func parseSystemdShow(raw string) map[string]string {
	props := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if key == "" {
			continue
		}
		props[key] = value
	}
	return props
}

// AvailableService represents a systemd/launchd unit discovered on the host
// that is not yet tracked by Sentinel.
type AvailableService struct {
	Unit        string `json:"unit"`
	Description string `json:"description"`
	ActiveState string `json:"activeState"`
	Manager     string `json:"manager"`
	Scope       string `json:"scope"`
}

// DiscoverServices lists service units visible on the host that are not
// already tracked. On Linux it queries both --user and --system scopes.
func (m *Manager) DiscoverServices(ctx context.Context) ([]AvailableService, error) {
	tracked, err := m.ListServices(ctx)
	if err != nil {
		return nil, err
	}
	trackedUnits := make(map[string]bool, len(tracked))
	for _, s := range tracked {
		trackedUnits[strings.ToLower(s.Unit)] = true
	}

	manager := detectManager(m.goos)
	var out []AvailableService

	switch manager {
	case managerSystemd:
		for _, scope := range []string{scopeUser, scopeSystem} {
			units, err := m.discoverSystemdUnits(ctx, scope)
			if err != nil {
				slog.Warn("service discovery failed", "manager", "systemd", "scope", scope, "err", err)
			}
			for _, u := range units {
				if trackedUnits[strings.ToLower(u.Unit)] {
					continue
				}
				u.Manager = managerSystemd
				u.Scope = scope
				out = append(out, u)
			}
		}
	case managerLaunchd:
		units, err := m.discoverLaunchdUnits(ctx)
		if err != nil {
			slog.Warn("service discovery failed", "manager", "launchd", "err", err)
		}
		for _, u := range units {
			if trackedUnits[strings.ToLower(u.Unit)] {
				continue
			}
			u.Manager = managerLaunchd
			u.Scope = scopeUser
			out = append(out, u)
		}
	}

	return out, nil
}

func (m *Manager) discoverSystemdUnits(ctx context.Context, scope string) ([]AvailableService, error) {
	args := make([]string, 0, 8)
	if scope == scopeUser {
		args = append(args, "--user")
	}
	args = append(args, "list-units", "--type=service,timer", "--all", "--no-pager", "--no-legend", "--plain")
	raw, err := m.commandRunner(ctx, "systemctl", args...)
	if err != nil {
		return nil, err
	}

	var units []AvailableService
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		unit := fields[0]
		active := fields[2]
		desc := ""
		if len(fields) > 4 {
			desc = strings.Join(fields[4:], " ")
		}
		units = append(units, AvailableService{
			Unit:        unit,
			Description: desc,
			ActiveState: active,
		})
	}
	return units, nil
}

func (m *Manager) discoverLaunchdUnits(ctx context.Context) ([]AvailableService, error) {
	raw, err := m.commandRunner(ctx, "launchctl", "list")
	if err != nil {
		return nil, err
	}

	var units []AvailableService
	for i, line := range strings.Split(raw, "\n") {
		if i == 0 { // skip header
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		label := fields[2]
		state := "active"
		if fields[0] == "-" {
			state = "inactive"
		}
		units = append(units, AvailableService{
			Unit:        label,
			Description: label,
			ActiveState: state,
		})
	}
	return units, nil
}

// BrowsedService represents a service unit found on the host, enriched with
// tracking information when it matches a registered custom service.
type BrowsedService struct {
	Unit         string `json:"unit"`
	Description  string `json:"description"`
	ActiveState  string `json:"activeState"`
	EnabledState string `json:"enabledState"`
	Manager      string `json:"manager"`
	Scope        string `json:"scope"`
	Tracked      bool   `json:"tracked"`
	TrackedName  string `json:"trackedName,omitempty"`
}

// BrowseServices returns all service units discovered on the host, annotated
// with tracking info for units that are already registered in the store.
func (m *Manager) BrowseServices(ctx context.Context) ([]BrowsedService, error) {
	tracked, err := m.ListServices(ctx)
	if err != nil {
		return nil, err
	}

	type trackedInfo struct{ Name string }
	trackedMap := make(map[string]trackedInfo, len(tracked))
	for _, s := range tracked {
		key := strings.ToLower(s.Unit) + "\x00" + strings.ToLower(s.Scope)
		trackedMap[key] = trackedInfo{Name: s.Name}
	}

	manager := detectManager(m.goos)
	var result []BrowsedService
	seen := make(map[string]bool)

	switch manager {
	case managerSystemd:
		for _, scope := range []string{scopeUser, scopeSystem} {
			units, err := m.discoverSystemdUnits(ctx, scope)
			if err != nil {
				slog.Warn("service discovery failed", "manager", "systemd", "scope", scope, "err", err)
			}
			for _, u := range units {
				key := strings.ToLower(u.Unit) + "\x00" + strings.ToLower(scope)
				if seen[key] {
					continue
				}
				seen[key] = true
				bs := BrowsedService{
					Unit:        u.Unit,
					Description: u.Description,
					ActiveState: u.ActiveState,
					Manager:     managerSystemd,
					Scope:       scope,
				}
				if info, ok := trackedMap[key]; ok {
					bs.Tracked = true
					bs.TrackedName = info.Name
				}
				result = append(result, bs)
			}
		}
	case managerLaunchd:
		units, err := m.discoverLaunchdUnits(ctx)
		if err != nil {
			slog.Warn("service discovery failed", "manager", "launchd", "err", err)
		}
		for _, u := range units {
			key := strings.ToLower(u.Unit) + "\x00" + strings.ToLower(scopeUser)
			if seen[key] {
				continue
			}
			seen[key] = true
			bs := BrowsedService{
				Unit:        u.Unit,
				Description: u.Description,
				ActiveState: u.ActiveState,
				Manager:     managerLaunchd,
				Scope:       scopeUser,
			}
			if info, ok := trackedMap[key]; ok {
				bs.Tracked = true
				bs.TrackedName = info.Name
			}
			result = append(result, bs)
		}
	}

	// Inject tracked services that were not returned by discover (e.g. built-ins).
	for _, s := range tracked {
		key := strings.ToLower(s.Unit) + "\x00" + strings.ToLower(s.Scope)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, BrowsedService{
			Unit:         s.Unit,
			Description:  s.DisplayName,
			ActiveState:  s.ActiveState,
			EnabledState: s.EnabledState,
			Manager:      s.Manager,
			Scope:        s.Scope,
			Tracked:      true,
			TrackedName:  s.Name,
		})
	}

	return result, nil
}

// ActByUnit performs a service action using unit/scope/manager directly,
// without requiring the service to be tracked in the store.
func (m *Manager) ActByUnit(ctx context.Context, unit, scope, manager, action string) error {
	action = strings.ToLower(strings.TrimSpace(action))
	if !isValidAction(action) {
		return ErrInvalidAction
	}

	switch manager {
	case managerSystemd:
		return m.actSystemdUnit(ctx, scope, unit, action)
	case managerLaunchd:
		return m.actLaunchdUnit(ctx, scope, unit, action)
	default:
		return fmt.Errorf("unsupported service manager: %s", manager)
	}
}

func (m *Manager) actSystemdUnit(ctx context.Context, scope, unit, action string) error {
	args := make([]string, 0, 4)
	if strings.EqualFold(scope, scopeUser) {
		args = append(args, "--user")
	}
	args = append(args, action, unit)
	_, err := m.commandRunner(ctx, "systemctl", args...)
	if err != nil {
		return fmt.Errorf("systemd action failed: %w", err)
	}
	return nil
}

func (m *Manager) actLaunchdUnit(ctx context.Context, scope, unit, action string) error {
	target := launchdTarget(scope, m.uidFn, unit)

	switch action {
	case ActionStop:
		_, err := m.commandRunner(ctx, "launchctl", "bootout", target)
		if err != nil && !isLaunchdMissingJobError(err) {
			return fmt.Errorf("launchd stop failed: %w", err)
		}
		return nil
	case ActionStart, ActionRestart:
		if loaded, _ := m.isLaunchdLoaded(ctx, target); !loaded {
			return fmt.Errorf("launchd service %s is not loaded", unit)
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

// InspectByUnit inspects a service identified by unit/scope/manager directly,
// without requiring the service to be tracked in the store.
func (m *Manager) InspectByUnit(ctx context.Context, unit, scope, manager string) (ServiceInspect, error) {
	target := ServiceStatus{
		Unit:    unit,
		Scope:   scope,
		Manager: manager,
	}

	inspect := ServiceInspect{
		Service:   target,
		Summary:   fmt.Sprintf("unit=%s scope=%s", unit, scope),
		CheckedAt: m.nowFn().UTC().Format(time.RFC3339),
	}

	switch manager {
	case managerSystemd:
		props, output, err := m.inspectSystemd(ctx, target)
		if err != nil {
			return ServiceInspect{}, err
		}
		inspect.Properties = props
		inspect.Output = output
		if summary := strings.TrimSpace(buildInspectSummary(props)); summary != "" {
			inspect.Summary = summary
		}
	case managerLaunchd:
		tgt := launchdTarget(scope, m.uidFn, unit)
		out, err := m.commandRunner(ctx, "launchctl", "print", tgt)
		if err != nil {
			return ServiceInspect{}, fmt.Errorf("launchd inspect failed: %w", err)
		}
		inspect.Output = out
	default:
		return ServiceInspect{}, fmt.Errorf("unsupported service manager: %s", manager)
	}

	return inspect, nil
}

func buildInspectSummary(props map[string]string) string {
	if len(props) == 0 {
		return ""
	}
	load := strings.TrimSpace(props["LoadState"])
	active := strings.TrimSpace(props["ActiveState"])
	sub := strings.TrimSpace(props["SubState"])
	parts := make([]string, 0, 3)
	if load != "" {
		parts = append(parts, "load="+load)
	}
	if active != "" {
		parts = append(parts, "active="+active)
	}
	if sub != "" {
		parts = append(parts, "sub="+sub)
	}
	return strings.Join(parts, " ")
}
