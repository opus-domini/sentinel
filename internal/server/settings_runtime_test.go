package server

import (
	"context"
	"errors"
	"testing"

	"github.com/opus-domini/sentinel/internal/config"
)

func TestSettingsRuntimeAppliesSharedLiveFields(t *testing.T) {
	before := config.Default()
	before.Server.Timezone = "UTC"
	before.Server.Locale = "en-US"
	before.Server.Token = "shared-secret"
	runtime := newSettingsRuntime()
	runtime.initialize(before)

	after := before
	after.Server.Timezone = "America/Sao_Paulo"
	after.Server.Locale = "pt-BR"
	after.MCP.Enabled = true
	err := runtime.ApplyConfig(context.Background(), before, after, []string{
		config.FieldServerTimezone,
		config.FieldServerLocale,
		config.FieldMCPEnabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Timezone() != after.Server.Timezone ||
		runtime.Locale() != after.Server.Locale ||
		!runtime.Enabled() ||
		!runtime.TokenConfigured() {
		t.Fatalf(
			"runtime = timezone:%q locale:%q enabled:%t token:%t",
			runtime.Timezone(),
			runtime.Locale(),
			runtime.Enabled(),
			runtime.TokenConfigured(),
		)
	}
}

func TestSettingsRuntimeRejectsMCPEnableWithoutStartupToken(t *testing.T) {
	before := config.Default()
	runtime := newSettingsRuntime()
	runtime.initialize(before)
	after := before
	after.Server.Token = "newly-persisted-secret"
	after.MCP.Enabled = true

	err := runtime.ApplyConfig(
		context.Background(),
		before,
		after,
		[]string{config.FieldServerToken, config.FieldMCPEnabled},
	)
	if !errors.Is(err, errMCPRuntimeTokenRequired) {
		t.Fatalf("ApplyConfig() error = %v, want startup token requirement", err)
	}
	if runtime.Enabled() {
		t.Fatal("runtime enabled MCP without a startup token")
	}
}
