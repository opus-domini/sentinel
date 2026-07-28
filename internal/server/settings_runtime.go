package server

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"

	"github.com/opus-domini/sentinel/internal/config"
)

// settingsRuntime is the live settings adapter shared by metadata, Settings,
// and MCP. Fields not handled here remain restart-only.
type settingsRuntime struct {
	mu sync.RWMutex

	initialized     bool
	timezone        string
	locale          string
	mcpEnabled      bool
	tokenConfigured bool
}

func newSettingsRuntime() *settingsRuntime {
	return &settingsRuntime{}
}

func (s *settingsRuntime) initialize(cfg config.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initialized = true
	s.timezone = cfg.Server.Timezone
	s.locale = cfg.Server.Locale
	s.mcpEnabled = cfg.MCP.Enabled
	s.tokenConfigured = strings.TrimSpace(cfg.Server.Token) != ""
}

func (s *settingsRuntime) ApplyConfig(
	_ context.Context,
	_ config.Config,
	after config.Config,
	keys []string,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.initialized {
		return errors.New("live settings state is not initialized")
	}

	if slices.Contains(keys, config.FieldServerTimezone) {
		s.timezone = after.Server.Timezone
	}
	if slices.Contains(keys, config.FieldServerLocale) {
		s.locale = after.Server.Locale
	}
	if slices.Contains(keys, config.FieldMCPEnabled) {
		if !after.MCP.Enabled || s.tokenConfigured {
			s.mcpEnabled = after.MCP.Enabled
		}
	}
	return nil
}

func (s *settingsRuntime) Timezone() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.timezone
}

func (s *settingsRuntime) Locale() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.locale
}

func (s *settingsRuntime) Enabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mcpEnabled
}

func (s *settingsRuntime) TokenConfigured() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tokenConfigured
}
