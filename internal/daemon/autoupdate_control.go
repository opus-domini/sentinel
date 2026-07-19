package daemon

import (
	"fmt"
	"runtime"
)

var (
	autoUpdateControlOS      = runtime.GOOS
	autoUpdateStatusFn       = UserAutoUpdateStatusForScope
	autoUpdateRunSystemctlFn = runSystemctlSystem
	autoUpdateRunUserctlFn   = runSystemctlUser
	autoUpdateBootoutFn      = launchdBootout
	autoUpdateBootstrapFn    = launchdBootstrap
)

// PauseAutoUpdate stops the managed updater while deployment files move. It
// returns whether the timer was active so the caller can restore that state.
func PauseAutoUpdate(scopeRaw string) (bool, error) {
	scope, err := normalizeExplicitScope(scopeRaw)
	if err != nil {
		return false, err
	}
	if err := RequireScopeAccess(scope); err != nil {
		return false, err
	}
	status, err := autoUpdateStatusFn(scope)
	if err != nil {
		return false, err
	}
	if !status.ServiceUnitExists && !status.TimerUnitExists {
		return false, nil
	}
	wasActive := status.TimerActiveState == serviceStateActive || status.TimerActiveState == "running"
	if autoUpdateControlOS == launchdSupportedOS {
		if !wasActive {
			return false, nil
		}
		if err := autoUpdateBootoutFn(scope, launchdAutoUpdateLabel); err != nil {
			return false, fmt.Errorf("stop autoupdate job: %w", err)
		}
		return true, nil
	}
	stopArgs := []string{"stop"}
	if status.TimerUnitExists {
		stopArgs = append(stopArgs, userAutoUpdateTimerName)
	}
	if status.ServiceUnitExists {
		stopArgs = append(stopArgs, userAutoUpdateServiceName)
	}
	if scope == managerScopeSystem {
		if err := autoUpdateRunSystemctlFn(stopArgs...); err != nil {
			return false, fmt.Errorf("stop system autoupdate units: %w", err)
		}
		return wasActive, nil
	}
	if err := withSystemdUserBusHint(autoUpdateRunUserctlFn(stopArgs...)); err != nil {
		return false, fmt.Errorf("stop user autoupdate units: %w", err)
	}
	return wasActive, nil
}

// ResumeAutoUpdate restores a timer that was active before PauseAutoUpdate.
func ResumeAutoUpdate(scopeRaw string, wasActive bool) error {
	if !wasActive {
		return nil
	}
	scope, err := normalizeExplicitScope(scopeRaw)
	if err != nil {
		return err
	}
	if err := RequireScopeAccess(scope); err != nil {
		return err
	}
	if autoUpdateControlOS == launchdSupportedOS {
		path, err := userAutoUpdatePathLaunchdForScope(scope)
		if err != nil {
			return err
		}
		return autoUpdateBootstrapFn(scope, path, launchdAutoUpdateLabel)
	}
	if scope == managerScopeSystem {
		return autoUpdateRunSystemctlFn("start", userAutoUpdateTimerName)
	}
	return withSystemdUserBusHint(autoUpdateRunUserctlFn("start", userAutoUpdateTimerName))
}
