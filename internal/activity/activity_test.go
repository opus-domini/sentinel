package activity

import "testing"

func TestNormalizeSeverity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty defaults to info", raw: "", want: SeverityInfo},
		{name: "trims and lowercases", raw: " WARN ", want: SeverityWarn},
		{name: "warning alias", raw: "warning", want: SeverityWarn},
		{name: "err alias", raw: "ERR", want: SeverityError},
		{name: "unknown preserved lowercase", raw: "Critical", want: "critical"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := NormalizeSeverity(tt.raw); got != tt.want {
				t.Fatalf("NormalizeSeverity(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
