// Package mcpserver exposes Sentinel's tmux, runbook and host sensor control planes over MCP.
package mcpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/opus-domini/sentinel/internal/runbook"
	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/tmux"
	"github.com/opus-domini/sentinel/internal/tmuxlifecycle"
)

// Options are the Sentinel-owned dependencies used by MCP tools.
type Options struct {
	Version               string
	Attachments           *AttachmentManager
	Lifecycle             *tmuxlifecycle.Manager
	SessionUser           func(string) string
	KnownSessionUsers     func() []string
	RegisterSessionUser   func(string, string)
	UnregisterSessionUser func(string)
	Runbooks              *runbook.Manager
	// Metrics reads one host sample for the sensor tool. *services.Manager
	// satisfies it. It is an interface so an omitted dependency stays a true
	// nil and the handler's guard fires.
	Metrics metricsSource
}

// Server owns the official MCP handler and tmux attachment manager.
type Server struct {
	state       availability
	guard       *security.Guard
	attachments *AttachmentManager
	handler     http.Handler
}

type availability interface {
	Enabled() bool
}

// New constructs the official Streamable HTTP MCP server.
func New(state availability, guard *security.Guard, opts Options) *Server {
	toolset := &tools{
		guard:       guard,
		attachments: opts.Attachments,
		lifecycle:   opts.Lifecycle,
		serviceForUser: func(user string) tmuxService {
			return tmux.Service{User: user}
		},
		sessionUser:           opts.SessionUser,
		knownSessionUsers:     opts.KnownSessionUsers,
		registerSessionUser:   opts.RegisterSessionUser,
		unregisterSessionUser: opts.UnregisterSessionUser,
		runbooks:              opts.Runbooks,
		metrics:               opts.Metrics,
	}
	version := strings.TrimSpace(opts.Version)
	if version == "" {
		version = "dev"
	}
	sdkServer := mcp.NewServer(&mcp.Implementation{
		Name:    "sentinel",
		Version: version,
	}, nil)
	toolset.register(sdkServer)

	sdkHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return sdkServer },
		&mcp.StreamableHTTPOptions{
			Stateless:    true,
			JSONResponse: true,
			Logger:       slog.Default(),
			// Sentinel validates Origin and Bearer authentication before the SDK.
			// Reverse proxies may forward a public Host to the loopback listener.
			DisableLocalhostProtection: true,
		},
	)
	return &Server{
		state:       state,
		guard:       guard,
		attachments: opts.Attachments,
		handler:     sdkHandler,
	}
}

// ServeHTTP applies Sentinel availability, origin and Bearer authentication
// before delegating protocol handling to the official SDK.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.state == nil || !s.state.Enabled() {
		http.NotFound(w, r)
		return
	}
	if s.guard == nil || !s.guard.TokenRequired() {
		writeAuthError(w, "MCP requires server.token")
		return
	}
	if err := s.guard.CheckOrigin(r); err != nil {
		s.guard.LogOriginDenial(r, err)
		http.Error(w, "request origin is not allowed", http.StatusForbidden)
		return
	}
	if !s.guard.TokenMatches(bearerToken(r.Header.Get("Authorization"))) {
		writeAuthError(w, "missing or invalid Bearer token")
		return
	}
	s.handler.ServeHTTP(w, r)
}

// Shutdown releases MCP attachments. Tmux sessions remain running.
func (s *Server) Shutdown(_ context.Context) {
	if s != nil && s.attachments != nil {
		s.attachments.Close()
	}
}

func bearerToken(value string) string {
	scheme, token, ok := strings.Cut(strings.TrimSpace(value), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func writeAuthError(w http.ResponseWriter, message string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="sentinel-mcp"`)
	http.Error(w, message, http.StatusUnauthorized)
}

func boolPtr(value bool) *bool {
	return &value
}

func closedWorldAnnotations(readOnly, destructive, idempotent bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    readOnly,
		DestructiveHint: boolPtr(destructive),
		IdempotentHint:  idempotent,
		OpenWorldHint:   boolPtr(false),
	}
}

func toolError(message string, err error) error {
	if err == nil {
		return errors.New(message)
	}
	return errors.New(message + ": " + err.Error())
}
