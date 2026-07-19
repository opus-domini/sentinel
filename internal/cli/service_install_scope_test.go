package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/opus-domini/sentinel/internal/daemon"
)

func TestResolveInstallerScopePreservesExistingInstallation(t *testing.T) {
	original := resolveInstallScopeFn
	t.Cleanup(func() { resolveInstallScopeFn = original })
	resolveInstallScopeFn = func(scope string) (string, error) {
		if scope != optionAuto {
			t.Fatalf("scope = %q", scope)
		}
		return daemon.ScopeSystem, nil
	}

	resolved, err := resolveInstallerScope(&App{}, optionAuto, false)
	if err != nil || resolved != daemon.ScopeSystem {
		t.Fatalf("resolveInstallerScope() = %q, %v", resolved, err)
	}
}

func TestResolveInstallerScopeRequiresExplicitNonInteractiveChoice(t *testing.T) {
	original := resolveInstallScopeFn
	t.Cleanup(func() { resolveInstallScopeFn = original })
	resolveInstallScopeFn = func(string) (string, error) {
		return "", daemon.ErrInstallScopeRequired
	}

	_, err := resolveInstallerScope(&App{}, optionAuto, false)
	if err == nil || !strings.Contains(err.Error(), "INSTALL_SCOPE=user") {
		t.Fatalf("resolveInstallerScope() error = %v", err)
	}
}

func TestResolveInstallerScopePromptsForFreshInstallation(t *testing.T) {
	original := resolveInstallScopeFn
	t.Cleanup(func() { resolveInstallScopeFn = original })
	resolveInstallScopeFn = func(string) (string, error) {
		return "", daemon.ErrInstallScopeRequired
	}

	var output bytes.Buffer
	resolved, err := resolveInstallerScope(&App{Stdin: strings.NewReader("invalid\n2\n"), Stderr: &output}, optionAuto, true)
	if err != nil || resolved != daemon.ScopeSystem {
		t.Fatalf("resolveInstallerScope() = %q, %v", resolved, err)
	}
	for _, want := range []string{"User installation", "System installation", "Enter 1", "Select [1/2]"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("prompt missing %q:\n%s", want, output.String())
		}
	}
}

func TestResolveInstallerScopePropagatesDiscoveryFailure(t *testing.T) {
	original := resolveInstallScopeFn
	t.Cleanup(func() { resolveInstallScopeFn = original })
	want := errors.New("inspect services")
	resolveInstallScopeFn = func(string) (string, error) { return "", want }

	if _, err := resolveInstallerScope(&App{}, optionAuto, true); !errors.Is(err, want) {
		t.Fatalf("resolveInstallerScope() error = %v", err)
	}
}

func TestPromptInstallScopeAcceptsUserLabel(t *testing.T) {
	var output bytes.Buffer
	resolved, err := promptInstallScope(strings.NewReader("user\n"), &output)
	if err != nil || resolved != daemon.ScopeUser {
		t.Fatalf("promptInstallScope() = %q, %v", resolved, err)
	}
}

func TestPromptInstallScopeRejectsCanceledInput(t *testing.T) {
	if _, err := promptInstallScope(strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Fatal("promptInstallScope() error = nil")
	}
}
