// Package daemon installs and inspects Sentinel user services.
package daemon

import (
	"fmt"
	"runtime"
	"slices"
)

// Lifecycle action names accepted by Control.
const (
	actionStart   = "start"
	actionStop    = "stop"
	actionRestart = "restart"
	actionEnable  = "enable"
	actionDisable = "disable"
)

// ServiceActions lists the lifecycle actions Control accepts.
var ServiceActions = []string{actionStart, actionStop, actionRestart, actionEnable, actionDisable}

// Control runs a lifecycle action against a resolved deployment.
func Control(action, scopeRaw string) error {
	if !validServiceAction(action) {
		return fmt.Errorf("unknown service action: %s", action)
	}
	if runtime.GOOS == launchdSupportedOS {
		return controlUserLaunchd(action, scopeRaw)
	}
	if err := ensureServicePlatformSupported(); err != nil {
		return err
	}
	deployment, err := ResolveDeployment(scopeRaw)
	if err != nil {
		return err
	}
	scope := deployment.Scope
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
func controlUserLaunchd(action, scopeRaw string) error {
	if err := ensureLaunchdSupported(); err != nil {
		return err
	}
	deployment, err := ResolveDeployment(scopeRaw)
	if err != nil {
		return err
	}
	scope := deployment.Scope
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
