package tmux

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/opus-domini/sentinel/internal/userswitch"
)

// isolatedCreationSites are the only functions allowed to build a tmux command
// that can start the server. Each must reach exec through Service.createRun,
// which is systemd-run --scope locally and systemd-run --machine (KillMode=
// process) for a target user. Any other site would put the tmux server inside
// sentinel.service's cgroup, so stopping Sentinel would kill every pane on the
// host — the regression cb02184 fixed and CreateSessionWithID reintroduced.
// Service.CreateSessionWithID is absent on purpose: it builds no command of its
// own, delegating to createSessionWithIDVia with Service.createRun as runner.
var isolatedCreationSites = map[string]string{
	"createSessionWithIDVia": "runner parameter, always Service.createRun",
	"Service.CreateSession":  "Service.createRun",
}

// TestSessionCreationStaysIsolated fails when a new call site starts the tmux
// server outside the isolated runners. It reads the package's own source, so it
// catches a fourth creation path the moment it is added, which is how this class
// of bug got in: the guarded path was correct and a second one was written
// beside it.
func TestSessionCreationStaysIsolated(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	found := make(map[string]bool)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if parseErr != nil {
			t.Fatalf("ParseFile(%s) error = %v", name, parseErr)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			ident, ok := node.(*ast.Ident)
			if !ok || ident.Name != "cmdNewSession" {
				return true
			}
			enclosing := enclosingFunc(file, ident.Pos())
			if enclosing == "" {
				// The constant declaration itself.
				return true
			}
			found[enclosing] = true
			if _, allowed := isolatedCreationSites[enclosing]; !allowed {
				t.Errorf(
					"%s builds a tmux new-session command but is not a known isolated creation site.\n"+
						"A command that can start the tmux server must run through createSessionRun or Service.run,\n"+
						"or the server inherits sentinel.service's cgroup and every pane dies with Sentinel.\n"+
						"Route it through an isolated runner, then add it to isolatedCreationSites.",
					enclosing,
				)
			}
			return true
		})
	}
	for site := range isolatedCreationSites {
		if !found[site] {
			t.Errorf("isolatedCreationSites lists %s, but no such site builds a new-session command any more; drop the stale entry", site)
		}
	}
}

// TestUserSessionCreationWrapsInSystemdRun pins the argv of both creation
// methods for a target user. Service.createRun sends them through the user
// switch method, whose systemd-run --machine wrapper carries
// KillMode=process — without that wrapper a tmux server started for another
// account dies with the transient unit that spawned it.
func TestUserSessionCreationWrapsInSystemdRun(t *testing.T) {
	// Not parallel: mutates execCommandContext, UserSwitchMethod and SystemUsers.

	originalUsers := SystemUsers
	t.Cleanup(func() { SystemUsers = originalUsers })
	SystemUsers = []string{"testuser"}

	originalMethod := UserSwitchMethod
	t.Cleanup(func() { UserSwitchMethod = originalMethod })
	UserSwitchMethod = userswitch.MethodSystemdRun

	var argv []string
	originalExec := execCommandContext
	t.Cleanup(func() { execCommandContext = originalExec })
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		argv = append([]string{name}, args...)
		// Echo a create-session identity so CreateSessionWithID still parses.
		cmd := exec.CommandContext(ctx, os.Args[0],
			"-test.run=TestExecCommandRecorder", "--", "$17"+fieldSep+"agent")
		cmd.Env = append(os.Environ(), "SENTINEL_EXEC_COMMAND_RECORDER=1")
		return cmd
	}

	svc := Service{User: "testuser"}
	tests := []struct {
		name     string
		call     func(*testing.T)
		tmuxArgs []string
	}{
		{
			name: "CreateSession",
			call: func(t *testing.T) {
				if err := svc.CreateSession(context.Background(), "agent", "/srv/app"); err != nil {
					t.Fatalf("CreateSession() error = %v", err)
				}
			},
			tmuxArgs: []string{"new-session", "-d", "-s", "agent", "-c", "/srv/app"},
		},
		{
			name: "CreateSessionWithID",
			call: func(t *testing.T) {
				if _, err := svc.CreateSessionWithID(context.Background(), "agent", "/srv/app"); err != nil {
					t.Fatalf("CreateSessionWithID() error = %v", err)
				}
			},
			tmuxArgs: []string{
				"new-session", "-d", "-P", "-F", createSessionFormat, "-s", "agent", "-c", "/srv/app",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argv = nil
			tt.call(t)
			want := append([]string{
				"sudo",
				"-n",
				"systemd-run",
				"--user",
				"--machine=testuser@.host",
				"--collect",
				"--quiet",
				"--service-type=exec",
				"--expand-environment=no",
				"--property=KillMode=process",
				"--wait",
				"--pipe",
				"tmux",
			}, tt.tmuxArgs...)
			if !slices.Equal(argv, want) {
				t.Fatalf("%s argv = %#v, want %#v", tt.name, argv, want)
			}
		})
	}
}

// enclosingFunc names the function or method containing pos, as "Name" or
// "Receiver.Name".
func enclosingFunc(file *ast.File, pos token.Pos) string {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || pos < fn.Pos() || pos > fn.End() {
			continue
		}
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			return receiverName(fn.Recv.List[0].Type) + "." + fn.Name.Name
		}
		return fn.Name.Name
	}
	return ""
}

func receiverName(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.StarExpr:
		return receiverName(typed.X)
	case *ast.Ident:
		return typed.Name
	}
	return ""
}
