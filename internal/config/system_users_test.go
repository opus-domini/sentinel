package config

import (
	"os"
	"testing"
)

func TestParsePasswd(t *testing.T) {
	t.Parallel()

	content := `root:x:0:0:root:/root:/bin/bash
daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin
bin:x:2:2:bin:/bin:/usr/sbin/nologin
sys:x:3:3:sys:/dev:/usr/sbin/nologin
nobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin
hugo:x:1000:1000:Hugo,,,:/home/hugo:/bin/bash
deploy:x:1001:1001::/home/deploy:/bin/zsh
postgres:x:999:999:PostgreSQL administrator:/var/lib/postgresql:/bin/bash
nologin-user:x:1002:1002::/home/nologin:/sbin/nologin
false-user:x:1003:1003::/home/false:/bin/false
`

	f := writeTempPasswd(t, content)

	users := parsePasswd(f)

	want := map[string]bool{
		"root":   true,
		"hugo":   true,
		"deploy": true,
	}

	if len(users) != len(want) {
		t.Fatalf("parsePasswd returned %v, want keys %v", users, want)
	}
	for _, u := range users {
		if !want[u] {
			t.Errorf("unexpected user %q in result", u)
		}
	}
}

func TestParsePasswdEmpty(t *testing.T) {
	t.Parallel()

	f := writeTempPasswd(t, "")
	users := parsePasswd(f)
	if len(users) != 0 {
		t.Errorf("parsePasswd on empty input = %v, want empty", users)
	}
}

func TestParsePasswdCommentsAndBlankLines(t *testing.T) {
	t.Parallel()

	content := `# This is a comment

hugo:x:1000:1000:Hugo:/home/hugo:/bin/bash
# Another comment
`

	f := writeTempPasswd(t, content)
	users := parsePasswd(f)

	if len(users) != 1 || users[0] != "hugo" {
		t.Errorf("parsePasswd = %v, want [hugo]", users)
	}
}

func TestParsePasswdMalformedLines(t *testing.T) {
	t.Parallel()

	content := `short:x:1000
baduid:x:notanumber:1000::/home/bad:/bin/bash
hugo:x:1000:1000:Hugo:/home/hugo:/bin/bash
`

	f := writeTempPasswd(t, content)
	users := parsePasswd(f)

	if len(users) != 1 || users[0] != "hugo" {
		t.Errorf("parsePasswd = %v, want [hugo]", users)
	}
}

func TestReadSystemUsersWithMockPasswd(t *testing.T) {
	// Not parallel: mutates package-level readPasswdFile.

	content := `root:x:0:0:root:/root:/bin/bash
daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin
hugo:x:1000:1000:Hugo:/home/hugo:/bin/bash
`
	tmpFile := writeTempPasswdPath(t, content)

	original := readPasswdFile
	t.Cleanup(func() { readPasswdFile = original })
	readPasswdFile = func() (*os.File, error) {
		return os.Open(tmpFile) //nolint:gosec // test-only, path is controlled
	}

	users := ReadSystemUsers()
	if len(users) == 0 {
		t.Fatal("ReadSystemUsers returned empty slice")
	}

	found := make(map[string]bool, len(users))
	for _, u := range users {
		found[u] = true
	}
	if !found["root"] {
		t.Error("expected root in system users")
	}
	if !found["hugo"] {
		t.Error("expected hugo in system users")
	}

	// Verify sorted.
	for i := 1; i < len(users); i++ {
		if users[i] < users[i-1] {
			t.Errorf("users not sorted: %v", users)
			break
		}
	}
}

func TestReadSystemUsersPasswdNotFound(t *testing.T) {
	// Not parallel: mutates package-level readPasswdFile.

	original := readPasswdFile
	t.Cleanup(func() { readPasswdFile = original })
	readPasswdFile = func() (*os.File, error) {
		return os.Open("/nonexistent/path/passwd")
	}

	users := ReadSystemUsers()
	// Should still include the current user as fallback.
	if len(users) == 0 {
		t.Error("ReadSystemUsers returned empty slice; expected at least the current user")
	}
}

func writeTempPasswd(t *testing.T, content string) *os.File {
	t.Helper()
	path := writeTempPasswdPath(t, content)
	f, err := os.Open(path) //nolint:gosec // test-only, path is controlled
	if err != nil {
		t.Fatalf("open temp passwd: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func writeTempPasswdPath(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "passwd-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		t.Fatalf("write temp file: %v", err)
	}
	name := f.Name()
	_ = f.Close()
	return name
}
