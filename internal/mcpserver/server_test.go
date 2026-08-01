package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/opus-domini/sentinel/internal/runbook"
	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/store"
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
	state := &testAvailability{}
	server := New(state, security.New("shared-token", nil, security.CookieSecureAuto), Options{
		Attachments: NewAttachmentManager(),
	})
	t.Cleanup(func() { server.Shutdown(context.Background()) })

	req := httptest.NewRequest(http.MethodPost, "http://sentinel.test/mcp", nil)
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("disabled status = %d, want 404", res.Code)
	}

	state.enabled = true
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
	st, err := store.New(filepath.Join(t.TempDir(), "sentinel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	runbooks := runbook.NewManager(st, nil, nil, 5, nil)
	t.Cleanup(func() { runbooks.Shutdown(context.Background()) })
	server := New(
		&testAvailability{enabled: true},
		security.New("shared-token", nil, security.CookieSecureAuto),
		Options{Version: "test", Attachments: NewAttachmentManager(), Runbooks: runbooks},
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
	if got := session.InitializeResult().ProtocolVersion; got != "2026-07-28" {
		t.Fatalf("negotiated protocol = %q, want 2026-07-28", got)
	}
	got := make([]string, 0, len(result.Tools))
	toolsByName := make(map[string]*mcp.Tool, len(result.Tools))
	for _, tool := range result.Tools {
		got = append(got, tool.Name)
		toolsByName[tool.Name] = tool
	}
	slices.Sort(got)
	want := []string{
		"runbook_create",
		"runbook_delete",
		"runbook_get",
		"runbook_get_run",
		"runbook_list",
		"runbook_list_runs",
		"runbook_run",
		"runbook_wait",
		"tmux_attach",
		"tmux_close_session",
		"tmux_create_session",
		"tmux_detach",
		"tmux_interact",
		"tmux_keep_session",
		"tmux_list_panes",
		"tmux_list_sessions",
		"tmux_list_windows",
		"tmux_read",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("tool names = %q, want %q", got, want)
	}
	keepAnnotations := toolsByName["tmux_keep_session"].Annotations
	if keepAnnotations == nil || keepAnnotations.ReadOnlyHint || keepAnnotations.DestructiveHint == nil ||
		*keepAnnotations.DestructiveHint {
		t.Fatalf("tmux_keep_session annotations = %#v", keepAnnotations)
	}
	closeAnnotations := toolsByName["tmux_close_session"].Annotations
	if closeAnnotations == nil || closeAnnotations.ReadOnlyHint || closeAnnotations.DestructiveHint == nil ||
		!*closeAnnotations.DestructiveHint {
		t.Fatalf("tmux_close_session annotations = %#v", closeAnnotations)
	}

	legacyInit := rawMCPPost(t, httpServer.URL, "", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"legacy-test","version":"test"}}}`)
	var initResponse struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"result"`
	}
	if err := json.Unmarshal(legacyInit, &initResponse); err != nil {
		t.Fatalf("decode legacy initialize: %v; body=%s", err, legacyInit)
	}
	if initResponse.Result.ProtocolVersion != "2025-11-25" {
		t.Fatalf("legacy negotiated protocol = %q; body=%s", initResponse.Result.ProtocolVersion, legacyInit)
	}
	legacyTools := rawMCPPost(t, httpServer.URL, "2025-11-25", `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	var toolsResponse struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(legacyTools, &toolsResponse); err != nil {
		t.Fatalf("decode legacy tools/list: %v; body=%s", err, legacyTools)
	}
	if len(toolsResponse.Result.Tools) == 0 {
		t.Fatalf("legacy tools/list returned no Sentinel tools; body=%s", legacyTools)
	}
}

func rawMCPPost(t *testing.T, endpoint, protocolVersion, body string) []byte {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer shared-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if protocolVersion != "" {
		req.Header.Set("MCP-Protocol-Version", protocolVersion)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("raw MCP status = %d; body=%s", response.StatusCode, payload)
	}
	return payload
}

type testAvailability struct {
	enabled bool
}

func (s *testAvailability) Enabled() bool {
	return s != nil && s.enabled
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
