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
	"sync"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
)

// customServicesRepo defines the store operations consumed by Manager.
type customServicesRepo interface {
	ListCustomServices(ctx context.Context) ([]store.CustomService, error)
}

const (
	// ServiceNameSentinel identifies the Sentinel service.
	ServiceNameSentinel = "sentinel"
	// ServiceNameUpdater identifies the Sentinel updater service.
	ServiceNameUpdater = "sentinel-updater"

	// ActionStart starts a managed service.
	ActionStart = "start"
	// ActionStop stops a managed service.
	ActionStop = "stop"
	// ActionRestart restarts a managed service.
	ActionRestart = "restart"
	// ActionEnable enables a managed service.
	ActionEnable = "enable"
	// ActionDisable disables a managed service.
	ActionDisable = "disable"

	scopeUser   = "user"
	scopeSystem = "system"

	managerSystemd = "systemd"
	managerLaunchd = "launchd"

	unitTypeService = "service"
	unitTypeJob     = "job"
	unitTypeUnit    = "unit"

	sentinelSystemdUnit = "sentinel"
	updaterSystemdUnit  = "sentinel-updater.timer"

	sentinelLaunchdLabel = "io.opusdomini.sentinel"
	updaterLaunchdLabel  = "io.opusdomini.sentinel.updater"

	stateActive   = "active"
	stateRunning  = "running"
	stateInactive = "inactive"
	stateFailed   = "failed"
	stateUnknown  = "unknown"
)

var (
	// ErrServiceNotFound is returned when an ops service cannot be found.
	ErrServiceNotFound = errors.New("ops service not found")
	// ErrInvalidAction is returned when an ops service action is not supported.
	ErrInvalidAction = errors.New("ops invalid action")
	// ErrInvalidUnit is returned when a service unit or launchd label is unsafe.
	ErrInvalidUnit = errors.New("ops invalid unit")

	systemdBrowseUnitTypes = []string{
		unitTypeService,
		"timer",
		"socket",
		"target",
		"path",
		"mount",
		"automount",
		"swap",
		"slice",
		"scope",
	}
)

type commandRunner func(ctx context.Context, name string, args ...string) (string, error)

// ServiceStatus represents service status data.
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

// ServiceInspect represents service inspect data.
type ServiceInspect struct {
	Service    ServiceStatus     `json:"service"`
	Summary    string            `json:"summary"`
	Properties map[string]string `json:"properties,omitempty"`
	Output     string            `json:"output,omitempty"`
	CheckedAt  string            `json:"checkedAt"`
}

// HostOverview represents host overview data.
type HostOverview struct {
	Hostname  string `json:"hostname"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	CPUs      int    `json:"cpus"`
	GoVersion string `json:"goVersion"`
}

// SentinelOverview represents sentinel overview data.
type SentinelOverview struct {
	PID       int   `json:"pid"`
	UptimeSec int64 `json:"uptimeSec"`
}

// Summary represents aggregate service state.
type Summary struct {
	Total  int `json:"total"`
	Active int `json:"active"`
	Failed int `json:"failed"`
}

// Overview represents overview data.
type Overview struct {
	Host      HostOverview     `json:"host"`
	Sentinel  SentinelOverview `json:"sentinel"`
	Services  Summary          `json:"services"`
	UpdatedAt string           `json:"updatedAt"`
}

// Manager represents manager data.
type Manager struct {
	startedAt      time.Time
	nowFn          func() time.Time
	hostname       func() (string, error)
	uidFn          func() int
	goos           string
	customServices customServicesRepo
	metricsMu      sync.Mutex
	metrics        *metricsCollector

	commandRunner commandRunner
}

// NewManager creates manager.
func NewManager(startedAt time.Time, csRepo customServicesRepo) *Manager {
	now := time.Now().UTC()
	if startedAt.IsZero() {
		startedAt = now
	}
	return &Manager{
		startedAt:      startedAt,
		nowFn:          time.Now,
		hostname:       os.Hostname,
		uidFn:          os.Getuid,
		goos:           runtime.GOOS,
		customServices: csRepo,
		metrics:        newMetricsCollector(),
		commandRunner:  runCommand,
	}
}

// Metrics returns value.
func (m *Manager) Metrics(ctx context.Context) HostMetrics {
	return m.metricsCollector().Collect(ctx, "/")
}

func (m *Manager) metricsCollector() *metricsCollector {
	if m == nil {
		return newMetricsCollector()
	}
	m.metricsMu.Lock()
	defer m.metricsMu.Unlock()
	if m.metrics == nil {
		m.metrics = newMetricsCollector()
	}
	return m.metrics
}

// Overview returns value.
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
		case stateFailed:
			out.Services.Failed++
		}
	}

	return out, nil
}

// ListServices lists services.
func (m *Manager) ListServices(ctx context.Context) ([]ServiceStatus, error) {
	now := m.nowFn().UTC().Format(time.RFC3339)
	var services []ServiceStatus

	if m.customServices != nil {
		custom, err := m.customServices.ListCustomServices(ctx)
		if err != nil {
			return nil, err
		}
		for _, cs := range custom {
			if !IsValidUnit(cs.Unit) {
				continue
			}
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
		if !IsValidUnit(label) {
			svc.Exists = false
			svc.ActiveState = stateUnknown
			svc.EnabledState = stateUnknown
			return
		}
		target := launchdTarget(svc.Scope, m.uidFn, label)
		out, err := m.commandRunner(ctx, "launchctl", "print", target)
		if err != nil {
			svc.Exists = false
			svc.ActiveState = stateInactive
			svc.EnabledState = "-"
			return
		}
		svc.Exists = true
		svc.ActiveState = launchdActiveState(out)
		svc.EnabledState = "enabled"
	default:
		svc.Exists = false
		svc.ActiveState = stateUnknown
		svc.EnabledState = stateUnknown
	}
}

// Act runs value.
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
		if err := m.actSystemd(ctx, target.Scope, target.Unit, action); err != nil {
			return ServiceStatus{}, err
		}
	case managerLaunchd:
		if err := m.actLaunchd(ctx, target.Scope, target.Unit, action); err != nil {
			return ServiceStatus{}, err
		}
	default:
		return ServiceStatus{}, fmt.Errorf("unsupported service manager: %s", target.Manager)
	}

	m.probeCustomService(ctx, &target)
	return target, nil
}

// Inspect inspects value.
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

func (m *Manager) actSystemd(ctx context.Context, scope, unit, action string) error {
	if !IsValidUnit(unit) {
		return ErrInvalidUnit
	}
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
	if !IsValidUnit(target.Unit) {
		return "", ErrInvalidUnit
	}
	out, err := m.commandRunner(ctx, "launchctl", "print", launchdTarget(target.Scope, m.uidFn, target.Unit))
	if err != nil {
		return "", fmt.Errorf("launchd inspect failed: %w", err)
	}
	return out, nil
}

func (m *Manager) actLaunchd(ctx context.Context, scope, label, action string) error {
	if !IsValidUnit(label) {
		return ErrInvalidUnit
	}
	target := launchdTarget(scope, m.uidFn, label)

	switch action {
	case ActionStop:
		_, err := m.commandRunner(ctx, "launchctl", "kill", "SIGTERM", target)
		if err != nil && !isLaunchdMissingJobError(err) {
			return fmt.Errorf("launchd stop failed: %w", err)
		}
		return nil
	case ActionStart, ActionRestart:
		if loaded, _ := m.isLaunchdLoaded(ctx, target); !loaded {
			return fmt.Errorf("launchd service %s is not loaded", label)
		}
		_, err := m.commandRunner(ctx, "launchctl", "kickstart", "-k", target)
		if err != nil {
			return fmt.Errorf("launchd %s failed: %w", action, err)
		}
		return nil
	case ActionEnable:
		_, err := m.commandRunner(ctx, "launchctl", "enable", target)
		if err != nil {
			return fmt.Errorf("launchd enable failed: %w", err)
		}
		return nil
	case ActionDisable:
		_, err := m.commandRunner(ctx, "launchctl", "disable", target)
		if err != nil {
			return fmt.Errorf("launchd disable failed: %w", err)
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

// systemdScopes returns the systemd scopes to query.
// When running as root (uid 0) there is no user D-Bus session,
// so only the system scope is returned.
func (m *Manager) systemdScopes() []string {
	if m.uidFn != nil && m.uidFn() == 0 {
		return []string{scopeSystem}
	}
	return []string{scopeUser, scopeSystem}
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
	if name != "" {
		return name, true
	}
	return "", false
}

// IsValidUnit reports whether a systemd unit or launchd label is safe to pass to commands.
func IsValidUnit(unit string) bool {
	unit = strings.TrimSpace(unit)
	if unit == "" || strings.HasPrefix(unit, "-") {
		return false
	}
	for _, r := range unit {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || strings.ContainsRune("._@:-", r) {
			continue
		}
		return false
	}
	return true
}

func launchdActiveState(raw string) string {
	props := parseLaunchdPrint(raw)
	state := strings.ToLower(strings.TrimSpace(props["state"]))
	exit := strings.TrimSpace(props["last exit code"])
	switch state {
	case stateRunning, stateActive:
		return stateRunning
	case "", stateInactive, "waiting", "exited", "stopped", "not running":
		if exit != "" && exit != "0" {
			return stateFailed
		}
		return stateInactive
	default:
		if exit != "" && exit != "0" {
			return stateFailed
		}
		return stateUnknown
	}
}

func parseLaunchdPrint(raw string) map[string]string {
	props := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, ";"))
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:idx]))
		value := strings.Trim(strings.TrimSpace(line[idx+1:]), `"`)
		props[key] = value
	}
	return props
}

func findServiceStatus(list []ServiceStatus, name string) (ServiceStatus, bool) {
	for _, item := range list {
		if item.Name == name {
			return item, true
		}
	}
	return ServiceStatus{}, false
}

func serviceKey(manager, scope, unit string) string {
	return strings.ToLower(strings.TrimSpace(manager)) + "\x00" + strings.ToLower(strings.TrimSpace(scope)) + "\x00" + strings.ToLower(strings.TrimSpace(unit))
}

func isValidAction(action string) bool {
	switch action {
	case ActionStart, ActionStop, ActionRestart, ActionEnable, ActionDisable:
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

// AvailableService represents a manageable systemd/launchd unit discovered on
// the host that is not yet tracked by Sentinel.
type AvailableService struct {
	Unit         string `json:"unit"`
	UnitType     string `json:"unitType"`
	Description  string `json:"description"`
	ActiveState  string `json:"activeState"`
	EnabledState string `json:"enabledState"`
	Manager      string `json:"manager"`
	Scope        string `json:"scope"`
}

// DiscoverServices lists manageable units visible on the host that are not
// already tracked. On Linux it queries both --user and --system scopes.
func (m *Manager) DiscoverServices(ctx context.Context) ([]AvailableService, error) {
	tracked, err := m.ListServices(ctx)
	if err != nil {
		return nil, err
	}
	trackedUnits := make(map[string]bool, len(tracked))
	for _, s := range tracked {
		trackedUnits[serviceKey(s.Manager, s.Scope, s.Unit)] = true
	}

	manager := detectManager(m.goos)
	var out []AvailableService

	switch manager {
	case managerSystemd:
		for _, scope := range m.systemdScopes() {
			units, err := m.discoverSystemdUnits(ctx, scope)
			if err != nil {
				slog.Warn("service discovery failed", "manager", "systemd", "scope", scope, "err", err)
			}
			for _, u := range units {
				if trackedUnits[serviceKey(managerSystemd, scope, u.Unit)] {
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
			scope := scopeUser
			if m.uidFn != nil && m.uidFn() == 0 {
				scope = scopeSystem
			}
			if trackedUnits[serviceKey(managerLaunchd, scope, u.Unit)] {
				continue
			}
			u.Manager = managerLaunchd
			u.Scope = scope
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
	args = append(
		args,
		"list-units",
		"--type="+strings.Join(systemdBrowseUnitTypes, ","),
		"--all",
		"--no-pager",
		"--no-legend",
		"--plain",
	)
	raw, err := m.commandRunner(ctx, "systemctl", args...)
	if err != nil {
		return nil, err
	}

	var units []AvailableService
	indexByUnit := make(map[string]int)
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
			Unit:         unit,
			UnitType:     browseUnitType(managerSystemd, unit),
			Description:  desc,
			ActiveState:  active,
			EnabledState: stateUnknown,
		})
		indexByUnit[strings.ToLower(unit)] = len(units) - 1
	}

	fileUnits, err := m.discoverSystemdUnitFiles(ctx, scope)
	if err != nil {
		slog.Warn("systemd unit-file discovery failed", "scope", scope, "err", err)
		return units, nil
	}

	for _, item := range fileUnits {
		key := strings.ToLower(item.Unit)
		if idx, ok := indexByUnit[key]; ok {
			units[idx].EnabledState = item.EnabledState
			continue
		}
		units = append(units, item)
		indexByUnit[key] = len(units) - 1
	}
	return units, nil
}

func (m *Manager) discoverSystemdUnitFiles(ctx context.Context, scope string) ([]AvailableService, error) {
	args := make([]string, 0, 8)
	if scope == scopeUser {
		args = append(args, "--user")
	}
	args = append(
		args,
		"list-unit-files",
		"--type="+strings.Join(systemdBrowseUnitTypes, ","),
		"--all",
		"--no-pager",
		"--no-legend",
	)
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
		if len(fields) < 2 {
			continue
		}
		unit := fields[0]
		units = append(units, AvailableService{
			Unit:         unit,
			UnitType:     browseUnitType(managerSystemd, unit),
			Description:  unit,
			ActiveState:  stateInactive,
			EnabledState: normalizeState(fields[1]),
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
			state = stateInactive
		}
		units = append(units, AvailableService{
			Unit:        label,
			UnitType:    browseUnitType(managerLaunchd, label),
			Description: label,
			ActiveState: state,
		})
	}
	return units, nil
}

// BrowsedService represents a manageable unit found on the host, enriched with
// tracking information when it matches a registered custom daemon.
type BrowsedService struct {
	Unit         string `json:"unit"`
	UnitType     string `json:"unitType"`
	Description  string `json:"description"`
	ActiveState  string `json:"activeState"`
	EnabledState string `json:"enabledState"`
	Manager      string `json:"manager"`
	Scope        string `json:"scope"`
	Tracked      bool   `json:"tracked"`
	TrackedName  string `json:"trackedName,omitempty"`
}

// BrowseServices returns all manageable units discovered on the host,
// annotated with tracking info for units that are already registered in the
// store.
func (m *Manager) BrowseServices(ctx context.Context) ([]BrowsedService, error) {
	tracked, err := m.ListServices(ctx)
	if err != nil {
		return nil, err
	}

	type trackedInfo struct{ Name string }
	trackedMap := make(map[string]trackedInfo, len(tracked))
	for _, s := range tracked {
		key := serviceKey(s.Manager, s.Scope, s.Unit)
		trackedMap[key] = trackedInfo{Name: s.Name}
	}

	manager := detectManager(m.goos)
	var result []BrowsedService
	seen := make(map[string]bool)

	switch manager {
	case managerSystemd:
		for _, scope := range m.systemdScopes() {
			units, err := m.discoverSystemdUnits(ctx, scope)
			if err != nil {
				slog.Warn("service discovery failed", "manager", "systemd", "scope", scope, "err", err)
			}
			for _, u := range units {
				key := serviceKey(managerSystemd, scope, u.Unit)
				if seen[key] {
					continue
				}
				seen[key] = true
				bs := BrowsedService{
					Unit:         u.Unit,
					UnitType:     u.UnitType,
					Description:  u.Description,
					ActiveState:  u.ActiveState,
					EnabledState: u.EnabledState,
					Manager:      managerSystemd,
					Scope:        scope,
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
			scope := scopeUser
			if m.uidFn != nil && m.uidFn() == 0 {
				scope = scopeSystem
			}
			key := serviceKey(managerLaunchd, scope, u.Unit)
			if seen[key] {
				continue
			}
			seen[key] = true
			bs := BrowsedService{
				Unit:         u.Unit,
				UnitType:     u.UnitType,
				Description:  u.Description,
				ActiveState:  u.ActiveState,
				EnabledState: u.EnabledState,
				Manager:      managerLaunchd,
				Scope:        scope,
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
		key := serviceKey(s.Manager, s.Scope, s.Unit)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, BrowsedService{
			Unit:         s.Unit,
			UnitType:     browseUnitType(s.Manager, s.Unit),
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
	if !IsValidUnit(unit) {
		return ErrInvalidUnit
	}
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
		_, err := m.commandRunner(ctx, "launchctl", "kill", "SIGTERM", target)
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
	case ActionEnable:
		_, err := m.commandRunner(ctx, "launchctl", "enable", target)
		if err != nil {
			return fmt.Errorf("launchd enable failed: %w", err)
		}
		return nil
	case ActionDisable:
		_, err := m.commandRunner(ctx, "launchctl", "disable", target)
		if err != nil {
			return fmt.Errorf("launchd disable failed: %w", err)
		}
		return nil
	default:
		return ErrInvalidAction
	}
}

// InspectByUnit inspects a service identified by unit/scope/manager directly,
// without requiring the service to be tracked in the store.
func (m *Manager) InspectByUnit(ctx context.Context, unit, scope, manager string) (ServiceInspect, error) {
	if !IsValidUnit(unit) {
		return ServiceInspect{}, ErrInvalidUnit
	}
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

func browseUnitType(manager, unit string) string {
	switch {
	case strings.EqualFold(manager, managerLaunchd):
		return unitTypeJob
	case !strings.EqualFold(manager, managerSystemd):
		return unitTypeUnit
	}

	trimmed := strings.TrimSpace(unit)
	if trimmed == "" {
		return unitTypeService
	}
	if idx := strings.LastIndexByte(trimmed, '.'); idx > 0 && idx < len(trimmed)-1 {
		return strings.ToLower(trimmed[idx+1:])
	}
	return unitTypeService
}
