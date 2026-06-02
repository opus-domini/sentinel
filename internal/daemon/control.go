// Package daemon installs and inspects Sentinel user services.
package daemon

import (
	"fmt"
	"runtime"
	"slices"
)

// Lifecycle action names accepted by ControlUser.
const (
	actionStart   = "start"
	actionStop    = "stop"
	actionRestart = "restart"
	actionEnable  = "enable"
	actionDisable = "disable"
)

// ServiceActions lists the lifecycle actions ControlUser accepts.
var ServiceActions = []string{actionStart, actionStop, actionRestart, actionEnable, actionDisable}

// ControlUser runs a lifecycle action on the managed Sentinel service. The
// action must be one of start, stop, restart, enable or disable. On Linux it
// drives systemctl (the system manager when run as root, otherwise the user
// manager); on macOS it drives launchctl.
func ControlUser(action string) error {
	if !validServiceAction(action) {
		return fmt.Errorf("unknown service action: %s", action)
	}
	if runtime.GOOS == launchdSupportedOS {
		return controlUserLaunchd(action)
	}
	if err := ensureServicePlatformSupported(); err != nil {
		return err
	}
	scope, err := resolveServiceScope()
	if err != nil {
		return err
	}
	if err := requireScopePrivilege(scope); err != nil {
		return err
	}
	if scope == managerScopeSystem {
		return runSystemctlSystem(action, "sentinel")
	}
	if err := ensureSystemdUserSupported(); err != nil {
		return err
	}
	return withSystemdUserBusHint(runSystemctlUser(action, "sentinel"))
}

func validServiceAction(action string) bool {
	return slices.Contains(ServiceActions, action)
}

// controlUserLaunchd maps a lifecycle action onto launchctl. launchd has no
// 1:1 equivalent of every systemd verb, so start/restart go through
// bootstrap+kickstart and stop unloads the job.
func controlUserLaunchd(action string) error {
	if err := ensureLaunchdSupported(); err != nil {
		return err
	}
	scope, err := resolveServiceScope()
	if err != nil {
		return err
	}
	if err := requireScopePrivilege(scope); err != nil {
		return err
	}
	switch action {
	case actionStart:
		servicePath, err := userServicePathLaunchdForScope(scope)
		if err != nil {
			return err
		}
		if err := launchdBootstrap(scope, servicePath, launchdServiceLabel); err != nil {
			return err
		}
		return launchdKickstart(scope, launchdServiceLabel)
	case actionStop:
		return launchdBootout(scope, launchdServiceLabel)
	case actionRestart:
		servicePath, err := userServicePathLaunchdForScope(scope)
		if err != nil {
			return err
		}
		// Bootstrap if the job isn't loaded (best effort: an already-loaded job
		// errors here, which kickstart then handles), so restart also works on a
		// stopped/unloaded service instead of failing in kickstart.
		_ = launchdBootstrap(scope, servicePath, launchdServiceLabel)
		return launchdKickstart(scope, launchdServiceLabel)
	case actionEnable:
		return runLaunchctl(actionEnable, launchdJobTarget(scope, launchdServiceLabel))
	case actionDisable:
		return runLaunchctl(actionDisable, launchdJobTarget(scope, launchdServiceLabel))
	default:
		return fmt.Errorf("unknown service action: %s", action)
	}
}
