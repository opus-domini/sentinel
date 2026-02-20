package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePositiveFloat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantVal float64
		wantOK  bool
	}{
		{"valid_integer", "95", 95.0, true},
		{"valid_decimal", "90.5", 90.5, true},
		{"valid_small", "0.1", 0.1, true},
		{"with_spaces", "  85.0  ", 85.0, true},

		{"zero", "0", 0, false},
		{"negative", "-1.5", 0, false},
		{"empty", "", 0, false},
		{"not_a_number", "abc", 0, false},
		{"negative_zero", "-0.0", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			val, ok := parsePositiveFloat(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("parsePositiveFloat(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if ok && val != tt.wantVal {
				t.Fatalf("parsePositiveFloat(%q) = %f, want %f", tt.input, val, tt.wantVal)
			}
		})
	}
}

func TestAlertThresholdsFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_ALERT_CPU_PERCENT", "85.5")
	t.Setenv("SENTINEL_ALERT_MEM_PERCENT", "75.0")
	t.Setenv("SENTINEL_ALERT_DISK_PERCENT", "80.0")
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_LOG_LEVEL", "")
	t.Setenv("SENTINEL_COOKIE_SECURE", "")
	t.Setenv("SENTINEL_ALLOW_INSECURE_COOKIE", "")

	cfg := Load()
	if cfg.AlertThresholds.CPUPercent != 85.5 {
		t.Fatalf("CPUPercent = %f, want 85.5", cfg.AlertThresholds.CPUPercent)
	}
	if cfg.AlertThresholds.MemPercent != 75.0 {
		t.Fatalf("MemPercent = %f, want 75.0", cfg.AlertThresholds.MemPercent)
	}
	if cfg.AlertThresholds.DiskPercent != 80.0 {
		t.Fatalf("DiskPercent = %f, want 80.0", cfg.AlertThresholds.DiskPercent)
	}
}

func TestAlertThresholdsFromFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := `alert_cpu_percent = 70.0
alert_mem_percent = 65.0
alert_disk_percent = 85.0
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_ALERT_CPU_PERCENT", "")
	t.Setenv("SENTINEL_ALERT_MEM_PERCENT", "")
	t.Setenv("SENTINEL_ALERT_DISK_PERCENT", "")
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_LOG_LEVEL", "")
	t.Setenv("SENTINEL_COOKIE_SECURE", "")
	t.Setenv("SENTINEL_ALLOW_INSECURE_COOKIE", "")

	cfg := Load()
	if cfg.AlertThresholds.CPUPercent != 70.0 {
		t.Fatalf("CPUPercent = %f, want 70.0", cfg.AlertThresholds.CPUPercent)
	}
	if cfg.AlertThresholds.MemPercent != 65.0 {
		t.Fatalf("MemPercent = %f, want 65.0", cfg.AlertThresholds.MemPercent)
	}
	if cfg.AlertThresholds.DiskPercent != 85.0 {
		t.Fatalf("DiskPercent = %f, want 85.0", cfg.AlertThresholds.DiskPercent)
	}
}

func TestAlertThresholdsInvalidFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_ALERT_CPU_PERCENT", "not-a-number")
	t.Setenv("SENTINEL_ALERT_MEM_PERCENT", "-10")
	t.Setenv("SENTINEL_ALERT_DISK_PERCENT", "0")
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_LOG_LEVEL", "")
	t.Setenv("SENTINEL_COOKIE_SECURE", "")
	t.Setenv("SENTINEL_ALLOW_INSECURE_COOKIE", "")

	cfg := Load()
	if cfg.AlertThresholds.CPUPercent != 90.0 {
		t.Fatalf("CPUPercent = %f, want 90.0 (default)", cfg.AlertThresholds.CPUPercent)
	}
	if cfg.AlertThresholds.MemPercent != 90.0 {
		t.Fatalf("MemPercent = %f, want 90.0 (default)", cfg.AlertThresholds.MemPercent)
	}
	if cfg.AlertThresholds.DiskPercent != 95.0 {
		t.Fatalf("DiskPercent = %f, want 95.0 (default)", cfg.AlertThresholds.DiskPercent)
	}
}

func TestRecoveryConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := `recovery_enabled = false
recovery_snapshot_interval = "10s"
recovery_max_snapshots = 500
recovery_capture_lines = 120
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_RECOVERY_ENABLED", "")
	t.Setenv("SENTINEL_RECOVERY_SNAPSHOT_INTERVAL", "")
	t.Setenv("SENTINEL_RECOVERY_MAX_SNAPSHOTS", "")
	t.Setenv("SENTINEL_RECOVERY_CAPTURE_LINES", "")
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_LOG_LEVEL", "")
	t.Setenv("SENTINEL_COOKIE_SECURE", "")
	t.Setenv("SENTINEL_ALLOW_INSECURE_COOKIE", "")

	cfg := Load()
	if cfg.Recovery.Enabled {
		t.Fatal("Recovery.Enabled = true, want false")
	}
	if cfg.Recovery.MaxSnapshots != 500 {
		t.Fatalf("MaxSnapshots = %d, want 500", cfg.Recovery.MaxSnapshots)
	}
	if cfg.Recovery.CaptureLines != 120 {
		t.Fatalf("CaptureLines = %d, want 120", cfg.Recovery.CaptureLines)
	}
}

func TestReadRawEnvOrFileNilMap(t *testing.T) {
	t.Setenv("TEST_RAW_NIL_MAP_KEY", "")
	got := readRawEnvOrFile("TEST_RAW_NIL_MAP_KEY", "key", nil)
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestParseBool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		wantV  bool
		wantOK bool
	}{
		{"true", "true", true, true},
		{"TRUE", "TRUE", true, true},
		{"yes", "yes", true, true},
		{"1", "1", true, true},
		{"on", "on", true, true},
		{"false", "false", false, true},
		{"FALSE", "FALSE", false, true},
		{"no", "no", false, true},
		{"0", "0", false, true},
		{"off", "off", false, true},
		{"invalid", "maybe", false, false},
		{"empty", "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v, ok := parseBool(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("parseBool(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if v != tt.wantV {
				t.Fatalf("parseBool(%q) = %v, want %v", tt.input, v, tt.wantV)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		wantOK bool
	}{
		{"valid_1s", "1s", true},
		{"valid_500ms", "500ms", true},
		{"valid_2m", "2m", true},
		{"zero", "0s", false},
		{"negative", "-1s", false},
		{"empty", "", false},
		{"garbage", "abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, ok := parseDuration(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("parseDuration(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
		})
	}
}

func TestParsePositiveInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		wantOK bool
	}{
		{"valid", "42", true},
		{"zero", "0", false},
		{"negative", "-5", false},
		{"empty", "", false},
		{"float", "3.14", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, ok := parsePositiveInt(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("parsePositiveInt(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
		})
	}
}
