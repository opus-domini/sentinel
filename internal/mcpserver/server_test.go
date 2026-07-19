package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/opus-domini/sentinel/internal/security"
)

func TestToolErrorAddsOperationContext(t *testing.T) {
	if got := toolError("list sessions", nil).Error(); got != "list sessions" {
		t.Fatalf("toolError() = %q", got)
	}
	if got := toolError("list sessions", errors.New("tmux unavailable")).Error(); got != "list sessions: tmux unavailable" {
		t.Fatalf("toolError() = %q", got)
	}
}

func TestServerAvailabilityAndBearerAuthentication(t *testing.T) {
	state := NewState(false, true)
	server := New(state, security.New("shared-token", nil, security.CookieSecureAuto), Options{})
	t.Cleanup(func() { server.Shutdown(context.Background()) })

	req := httptest.NewRequest(http.MethodPost, "http://sentinel.test/mcp", nil)
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("disabled status = %d, want 404", res.Code)
	}

	if err := state.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "http://sentinel.test/mcp", nil)
	req.AddCookie(&http.Cookie{Name: "sentinel_token", Value: "shared-token"})
	res = httptest.NewRecorder()
	server.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("cookie-only status = %d, want 401", res.Code)
	}
	if got := res.Header().Get("WWW-Authenticate"); got == "" {
		t.Fatal("401 response is missing WWW-Authenticate")
	}
}

func TestOfficialClientListsSentinelToolsBehindReverseProxy(t *testing.T) {
	server := New(
		NewState(true, true),
		security.New("shared-token", nil, security.CookieSecureAuto),
		Options{Version: "test"},
	)
	t.Cleanup(func() { server.Shutdown(context.Background()) })
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "sentinel-test", Version: "test"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: httpServer.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{
			token: "shared-token",
			host:  "azdrix.example.ts.net",
		}},
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("official client Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	got := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		got = append(got, tool.Name)
	}
	slices.Sort(got)
	want := []string{
		"tmux_attach",
		"tmux_create_session",
		"tmux_detach",
		"tmux_interact",
		"tmux_list_panes",
		"tmux_list_sessions",
		"tmux_list_windows",
		"tmux_read",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("tool names = %q, want %q", got, want)
	}
}

type bearerTransport struct {
	token string
	host  string
}

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	if t.host != "" {
		clone.Host = t.host
	}
	return http.DefaultTransport.RoundTrip(clone)
}
