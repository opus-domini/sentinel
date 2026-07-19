// Package cli implements the Sentinel command-line interface on top of cobra.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/daemon"
	"github.com/opus-domini/sentinel/internal/server"
	"github.com/opus-domini/sentinel/internal/updater"
)

// App is the per-invocation runtime context shared by every subcommand.
type App struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Shared command and output keys.
const (
	cmdStatus           = "status"
	cmdConfig           = "config"
	cmdInstall          = "install"
	optionAuto          = "auto"
	optionUser          = "user"
	optionSystem        = "system"
	hostOSDarwin        = "darwin"
	sentinelServiceUnit = "sentinel"
	stateActive         = "active"
	stateEnabled        = "enabled"
	stateRunning        = "running"
)

// Test indirections: swapped to fakes so command behaviour can be exercised
// without touching the real service manager, updater or HTTP server.
var (
	daemonFn                  = runDaemon
	installUserSvcFn          = daemon.InstallUser
	uninstallUserSvcFn        = daemon.UninstallUser
	controlScopedServiceFn    = daemon.Control
	serviceStatusFn           = daemon.ServiceStatus
	userLogsFn                = daemon.UserLogs
	installUserAutoUpdateFn   = daemon.InstallUserAutoUpdate
	reconcileAutoUpdateFn     = daemon.ReconcileAutoUpdate
	uninstallUserAutoUpdateFn = daemon.UninstallUserAutoUpdate
	userAutoUpdateStatusFn    = daemon.UserAutoUpdateStatusForScope
	removeShellCompletionsFn  = removeShellCompletions
	removeSentinelBinaryAtFn  = removeSentinelBinaryAt
	loadConfigFn              = config.Load
	loadConfigPathFn          = config.LoadPathForDataDir
	loadConfigDeploymentFn    = config.LoadPathForDeployment
	currentVersionFn          = Version
	updateCheckFn             = updater.Check
	updateApplyFn             = updater.Apply
	updateStatusFn            = updater.Status
	installedDeploymentsFn    = daemon.InstalledDeployments
	resolveDeploymentFn       = daemon.ResolveDeployment
	resolveInstallScopeFn     = daemon.ResolveInstallScope
	requireScopeAccessFn      = daemon.RequireScopeAccess
	pauseAutoUpdateFn         = daemon.PauseAutoUpdate
	resumeAutoUpdateFn        = daemon.ResumeAutoUpdate
)

// runDaemon is the default daemonFn: it boots the HTTP server with the
// resolved binary version.
func runDaemon() int {
	return server.Serve(currentVersionFn())
}

// Run parses args, dispatches to a Sentinel CLI command and returns the
// process exit code. With no args it prints the root help — starting the
// server requires the explicit "daemon" command.
func Run(args []string, stdout, stderr io.Writer) int {
	originalConfigPath, configPathWasSet := os.LookupEnv("SENTINEL_CONFIG")
	defer func() {
		if configPathWasSet {
			_ = os.Setenv("SENTINEL_CONFIG", originalConfigPath)
			return
		}
		_ = os.Unsetenv("SENTINEL_CONFIG")
	}()

	app := &App{Stdin: os.Stdin, Stdout: stdout, Stderr: stderr}
	root := newRootCmd(app)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	err := root.Execute()
	if err != nil {
		var ee exitError
		if errors.As(err, &ee) {
			if ee.err != nil {
				writeln(stderr, ee.err)
			}
		} else {
			writeln(stderr, err)
		}
	}
	return exitCode(err)
}

// exitError carries an explicit process exit code out of a command. A nil err
// means the failure was already reported elsewhere (e.g. the daemon logged it
// via slog), so Run prints nothing.
type exitError struct {
	code int
	err  error
}

func (e exitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e exitError) Unwrap() error { return e.err }

// failf builds an exitError carrying a formatted message.
func failf(format string, a ...any) error {
	return exitError{code: 1, err: fmt.Errorf(format, a...)}
}

// exitCode maps a command error to a process exit code. An exitError carries
// its own code; any other error is a cobra usage error (exit 2).
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	return 2
}
