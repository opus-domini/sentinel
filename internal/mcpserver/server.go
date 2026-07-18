// Package mcpserver exposes Sentinel's tmux control plane over MCP.
package mcpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/tmux"
)

// Options are the Sentinel-owned dependencies used by MCP tools.
type Options struct {
	Version             string
	SessionUser         func(string) string
	KnownSessionUsers   func() []string
	RegisterSessionUser func(string, string)
}

// Server owns the official MCP handler and tmux attachment manager.
type Server struct {
	state       *State
	guard       *security.Guard
	attachments *AttachmentManager
	handler     http.Handler
}

// New constructs the official Streamable HTTP MCP server.
func New(state *State, guard *security.Guard, opts Options) *Server {
	attachments := NewAttachmentManager()
	toolset := &tools{
		guard:       guard,
		attachments: attachments,
		serviceForUser: func(user string) tmuxService {
			return tmux.Service{User: user}
		},
		sessionUser:         opts.SessionUser,
		knownSessionUsers:   opts.KnownSessionUsers,
		registerSessionUser: opts.RegisterSessionUser,
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
		attachments: attachments,
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
