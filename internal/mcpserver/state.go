package mcpserver

import (
	"errors"
	"sync/atomic"
)

// ErrTokenRequired is returned when MCP is enabled without server.token.
var ErrTokenRequired = errors.New("server.token is required before MCP can be enabled")

// State owns the live MCP feature state shared by the HTTP and settings handlers.
type State struct {
	enabled         atomic.Bool
	tokenConfigured bool
}

// NewState creates live MCP state from the effective startup configuration.
func NewState(enabled, tokenConfigured bool) *State {
	state := &State{tokenConfigured: tokenConfigured}
	state.enabled.Store(enabled)
	return state
}

// Enabled reports whether /mcp is currently available.
func (s *State) Enabled() bool {
	return s != nil && s.enabled.Load()
}

// TokenConfigured reports whether the shared server token is configured.
func (s *State) TokenConfigured() bool {
	return s != nil && s.tokenConfigured
}

// SetEnabled changes availability without restarting Sentinel.
func (s *State) SetEnabled(enabled bool) error {
	if s == nil {
		return errors.New("MCP state is unavailable")
	}
	if enabled && !s.tokenConfigured {
		return ErrTokenRequired
	}
	s.enabled.Store(enabled)
	return nil
}
