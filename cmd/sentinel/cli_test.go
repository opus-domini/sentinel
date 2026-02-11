package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/service"
)

func TestRunCLIDefaultServe(t *testing.T) {
	origServe := serveFn
	t.Cleanup(func() { serveFn = origServe })

	called := false
	serveFn = func() int {
		called = true
		return 0
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI(nil, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !called {
		t.Fatal("serveFn was not called")
	}
}

func TestRunCLIServiceInstallParsesFlags(t *testing.T) {
	origInstall := installUserSvcFn
	t.Cleanup(func() { installUserSvcFn = origInstall })

	var got service.InstallUserOptions
	installUserSvcFn = func(opts service.InstallUserOptions) error {
		got = opts
		return nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"service", "install", "-exec", "/tmp/sentinel", "-enable=false", "-start=false"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if got.ExecPath != "/tmp/sentinel" {
		t.Fatalf("ExecPath = %q, want /tmp/sentinel", got.ExecPath)
	}
	if got.Enable {
		t.Fatal("Enable = true, want false")
	}
	if got.Start {
		t.Fatal("Start = true, want false")
	}
}

func TestRunCLIServiceStatus(t *testing.T) {
	origStatus := userStatusFn
	t.Cleanup(func() { userStatusFn = origStatus })

	userStatusFn = func() (service.UserServiceStatus, error) {
		return service.UserServiceStatus{
			ServicePath:        "/tmp/sentinel.service",
			UnitFileExists:     true,
			SystemctlAvailable: true,
			EnabledState:       "enabled",
			ActiveState:        "active",
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"service", "status"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	text := out.String()
	for _, fragment := range []string{
		"service file: /tmp/sentinel.service",
		"unit exists: true",
		"enabled: enabled",
		"active: active",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, text)
		}
	}
}

func TestRunCLIDoctor(t *testing.T) {
	origLoad := loadConfigFn
	origStatus := userStatusFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		userStatusFn = origStatus
	})

	loadConfigFn = func() config.Config {
		return config.Config{
			ListenAddr: "127.0.0.1:4040",
			DataDir:    "/tmp/.sentinel",
			Token:      "token",
		}
	}
	userStatusFn = func() (service.UserServiceStatus, error) {
		return service.UserServiceStatus{
			ServicePath:        "/tmp/sentinel.service",
			UnitFileExists:     true,
			EnabledState:       "enabled",
			ActiveState:        "active",
			SystemctlAvailable: true,
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"doctor"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	text := out.String()
	for _, fragment := range []string{
		"Sentinel doctor report",
		"listen: 127.0.0.1:4040",
		"data dir: /tmp/.sentinel",
		"token required: true",
		"user unit file: /tmp/sentinel.service",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, text)
		}
	}
}

func TestRunCLIUnknownCommand(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"unknown"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown command: unknown") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunCLIServiceInstallFailure(t *testing.T) {
	origInstall := installUserSvcFn
	t.Cleanup(func() { installUserSvcFn = origInstall })

	installUserSvcFn = func(_ service.InstallUserOptions) error {
		return errors.New("install failed")
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"service", "install"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "service install failed") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunCLIHelpFlag(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"-h"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "Sentinel command-line interface") {
		t.Fatalf("unexpected help output: %s", out.String())
	}
}

func TestRunCLIVersionFlag(t *testing.T) {
	origVersion := currentVersionFn
	t.Cleanup(func() { currentVersionFn = origVersion })
	currentVersionFn = func() string { return "v1.2.3" }

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCLI([]string{"--version"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(out.String()) != "sentinel version v1.2.3" {
		t.Fatalf("unexpected version output: %q", out.String())
	}
}
