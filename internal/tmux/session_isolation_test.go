package tmux

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolatedCreationSites are the only functions allowed to build a tmux command
// that can start the server. Each must reach exec through createSessionRun (the
// systemd-run wrapper) or through Service.run (systemd-run --machine, which
// also sets KillMode=process). Any other site would put the tmux server inside
// sentinel.service's cgroup, so stopping Sentinel would kill every pane on the
// host — the regression cb02184 fixed and CreateSessionWithID reintroduced.
// Service.CreateSessionWithID is absent on purpose: it builds no command of its
// own, delegating to createSessionWithIDVia with Service.run as the runner.
var isolatedCreationSites = map[string]string{
	"CreateSession":          "createSessionRun",
	"createSessionWithIDVia": "runner parameter, always createSessionRun or Service.run",
	"Service.CreateSession":  "Service.run",
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
