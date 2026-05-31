package sentinel

import "testing"

// TestVersionPrefersLdflags asserts that an injected ldflags value is returned
// verbatim, without trimming a leading "v".
func TestVersionPrefersLdflags(t *testing.T) {
	orig := version
	t.Cleanup(func() { version = orig })

	version = "1.9.0"
	if got := Version(); got != "1.9.0" {
		t.Fatalf("Version() = %q, want 1.9.0", got)
	}

	version = "v2.0.0"
	if got := Version(); got != "v2.0.0" {
		t.Fatalf("Version() = %q, want v2.0.0 (no v trimming)", got)
	}
}

// TestVersionFallback covers the paths where the ldflags value is a sentinel
// ("dev", "(devel)", empty). Version falls back to build info or "dev", but is
// always non-empty.
func TestVersionFallback(t *testing.T) {
	orig := version
	t.Cleanup(func() { version = orig })

	for _, sentinel := range []string{"dev", "(devel)", "", "   "} {
		version = sentinel
		// In the test binary debug.ReadBuildInfo may report "(devel)" for the
		// main module, so we only assert the value is non-empty; the resolver
		// never returns "".
		if got := Version(); got == "" {
			t.Fatalf("Version() returned empty string for version=%q", sentinel)
		}
	}
}
