package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultRepo         = "opus-domini/sentinel"
	defaultAPIBase      = "https://api.github.com"
	defaultServiceUnit  = "sentinel"
	binaryName          = "sentinel"
	maxExtractedBinSize = int64(128 * 1024 * 1024) // 128 MiB hard limit for extracted binary.
)

type CheckOptions struct {
	CurrentVersion string
	Repo           string
	APIBaseURL     string
	OS             string
	Arch           string
	DataDir        string
	HTTPClient     *http.Client
}

type CheckResult struct {
	CurrentVersion string
	LatestVersion  string
	UpToDate       bool
	ReleaseURL     string
	AssetName      string
	AssetURL       string
	ExpectedSHA256 string
	CheckedAt      time.Time
}

type ApplyOptions struct {
	CurrentVersion  string
	Repo            string
	APIBaseURL      string
	OS              string
	Arch            string
	DataDir         string
	ExecPath        string
	AllowDowngrade  bool
	AllowUnverified bool
	Restart         bool
	ServiceUnit     string
	SystemdScope    string // user, system, none
	HTTPClient      *http.Client
}

type ApplyResult struct {
	Applied        bool
	CurrentVersion string
	LatestVersion  string
	BinaryPath     string
	BackupPath     string
	CheckedAt      time.Time
	AppliedAt      time.Time
}

type State struct {
	LastCheckedAt      time.Time `json:"lastCheckedAt,omitempty"`
	LastAppliedAt      time.Time `json:"lastAppliedAt,omitempty"`
	CurrentVersion     string    `json:"currentVersion,omitempty"`
	LatestVersion      string    `json:"latestVersion,omitempty"`
	UpToDate           bool      `json:"upToDate"`
	LastError          string    `json:"lastError,omitempty"`
	LastReleaseURL     string    `json:"lastReleaseUrl,omitempty"`
	LastAppliedBinary  string    `json:"lastAppliedBinary,omitempty"`
	LastAppliedBackup  string    `json:"lastAppliedBackup,omitempty"`
	LastExpectedSHA256 string    `json:"lastExpectedSha256,omitempty"`
}

type release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []asset
}

type asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

func Check(ctx context.Context, opts CheckOptions) (CheckResult, error) {
	now := time.Now().UTC()
	cfg := normalizeCheckOptions(opts)

	rel, err := fetchLatestRelease(ctx, cfg)
	if err != nil {
		recordStateError(cfg.DataDir, now, cfg.CurrentVersion, err)
		return CheckResult{}, err
	}

	latestVersion := normalizeVersion(rel.TagName)
	if latestVersion == "" {
		err := fmt.Errorf("release tag is empty")
		recordStateError(cfg.DataDir, now, cfg.CurrentVersion, err)
		return CheckResult{}, err
	}

	archiveName := fmt.Sprintf("sentinel-%s-%s-%s.tar.gz", latestVersion, cfg.OS, cfg.Arch)
	archiveAsset, ok := findAssetByName(rel.Assets, archiveName)
	if !ok {
		err := fmt.Errorf("release %s does not provide asset %s", rel.TagName, archiveName)
		recordStateError(cfg.DataDir, now, cfg.CurrentVersion, err)
		return CheckResult{}, err
	}

	expectedSHA, err := resolveExpectedArchiveSHA256(ctx, cfg, rel.Assets, latestVersion, archiveName, archiveAsset)
	if err != nil {
		recordStateError(cfg.DataDir, now, cfg.CurrentVersion, err)
		return CheckResult{}, err
	}

	result := CheckResult{
		CurrentVersion: normalizeVersion(cfg.CurrentVersion),
		LatestVersion:  latestVersion,
		UpToDate:       isCurrentUpToDate(normalizeVersion(cfg.CurrentVersion), latestVersion),
		ReleaseURL:     rel.HTMLURL,
		AssetName:      archiveName,
		AssetURL:       archiveAsset.BrowserDownloadURL,
		ExpectedSHA256: expectedSHA,
		CheckedAt:      now,
	}
	if result.CurrentVersion == "" {
		result.CurrentVersion = normalizeVersion(cfg.CurrentVersion)
	}
	recordCheckState(cfg.DataDir, result)
	return result, nil
}

func Apply(ctx context.Context, opts ApplyOptions) (ApplyResult, error) {
	cfg := normalizeApplyOptions(opts)

	var out ApplyResult
	err := withUpdateLock(cfg.DataDir, func() error {
		check, err := Check(ctx, CheckOptions{
			CurrentVersion: cfg.CurrentVersion,
			Repo:           cfg.Repo,
			APIBaseURL:     cfg.APIBaseURL,
			OS:             cfg.OS,
			Arch:           cfg.Arch,
			DataDir:        cfg.DataDir,
			HTTPClient:     cfg.HTTPClient,
		})
		if err != nil {
			return err
		}

		out.CheckedAt = check.CheckedAt
		out.CurrentVersion = check.CurrentVersion
		out.LatestVersion = check.LatestVersion

		if check.UpToDate {
			out.Applied = false
			return nil
		}

		if !cfg.AllowDowngrade && compareVersions(check.CurrentVersion, check.LatestVersion) > 0 {
			return fmt.Errorf("current version (%s) is newer than latest release (%s)", check.CurrentVersion, check.LatestVersion)
		}

		expectedSHA := strings.ToLower(strings.TrimSpace(check.ExpectedSHA256))
		if expectedSHA == "" && !cfg.AllowUnverified {
			return errors.New("release asset checksum is unavailable; refusing unverified update")
		}

		tmpDir, err := os.MkdirTemp("", "sentinel-update-*")
		if err != nil {
			return fmt.Errorf("create temp dir: %w", err)
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()

		archivePath := filepath.Join(tmpDir, check.AssetName)
		if err := downloadToFile(ctx, cfg.HTTPClient, check.AssetURL, archivePath); err != nil {
			return fmt.Errorf("download release archive: %w", err)
		}

		archiveSHA, err := fileSHA256(archivePath)
		if err != nil {
			return fmt.Errorf("calculate archive checksum: %w", err)
		}
		if expectedSHA != "" && archiveSHA != expectedSHA {
			return fmt.Errorf("checksum mismatch for %s", check.AssetName)
		}

		execPath, err := resolveExecPath(cfg.ExecPath)
		if err != nil {
			return err
		}

		newBinaryPath := execPath + ".new"
		if err := extractBinaryFromArchive(archivePath, newBinaryPath); err != nil {
			return fmt.Errorf("extract binary: %w", err)
		}

		appliedAt := time.Now().UTC()
		backupPath := execPath + ".bak"
		if err := installBinary(execPath, newBinaryPath, backupPath); err != nil {
			return err
		}

		restartCmd := cfg.buildRestartCommand()
		if len(restartCmd) > 0 {
			if err := runCommand(ctx, restartCmd[0], restartCmd[1:]...); err != nil {
				_ = rollbackBinary(execPath, backupPath)
				return fmt.Errorf("restart service failed: %w", err)
			}
		}

		out.Applied = true
		out.AppliedAt = appliedAt
		out.BinaryPath = execPath
		out.BackupPath = backupPath

		recordApplyState(cfg.DataDir, check, out, "")
		return nil
	})
	if err != nil {
		recordStateError(cfg.DataDir, time.Now().UTC(), cfg.CurrentVersion, err)
		return ApplyResult{}, err
	}
	return out, nil
}

func Status(dataDir string) (State, error) {
	return loadState(dataDir)
}

func normalizeCheckOptions(opts CheckOptions) CheckOptions {
	cfg := opts
	cfg.Repo = strings.TrimSpace(cfg.Repo)
	if cfg.Repo == "" {
		cfg.Repo = defaultRepo
	}
	cfg.APIBaseURL = strings.TrimSpace(cfg.APIBaseURL)
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = defaultAPIBase
	}
	cfg.OS = strings.TrimSpace(cfg.OS)
	if cfg.OS == "" {
		cfg.OS = runtime.GOOS
	}
	cfg.Arch = strings.TrimSpace(cfg.Arch)
	if cfg.Arch == "" {
		cfg.Arch = runtime.GOARCH
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 20 * time.Second}
	}
	return cfg
}

func normalizeApplyOptions(opts ApplyOptions) ApplyOptions {
	cfg := opts
	cfg.Repo = strings.TrimSpace(cfg.Repo)
	if cfg.Repo == "" {
		cfg.Repo = defaultRepo
	}
	cfg.APIBaseURL = strings.TrimSpace(cfg.APIBaseURL)
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = defaultAPIBase
	}
	cfg.OS = strings.TrimSpace(cfg.OS)
	if cfg.OS == "" {
		cfg.OS = runtime.GOOS
	}
	cfg.Arch = strings.TrimSpace(cfg.Arch)
	if cfg.Arch == "" {
		cfg.Arch = runtime.GOARCH
	}
	cfg.SystemdScope = strings.ToLower(strings.TrimSpace(cfg.SystemdScope))
	if cfg.SystemdScope == "" {
		if runtime.GOOS == "linux" {
			cfg.SystemdScope = "user"
		} else {
			cfg.SystemdScope = "none"
		}
	}
	cfg.ServiceUnit = strings.TrimSpace(cfg.ServiceUnit)
	if cfg.ServiceUnit == "" {
		cfg.ServiceUnit = defaultServiceUnit
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	return cfg
}

func (o ApplyOptions) buildRestartCommand() []string {
	if !o.Restart {
		return nil
	}
	unit := strings.TrimSpace(o.ServiceUnit)
	if unit == "" {
		unit = defaultServiceUnit
	}
	switch o.SystemdScope {
	case "none":
		return nil
	case "system":
		return []string{"systemctl", "restart", unit}
	default:
		return []string{"systemctl", "--user", "restart", unit}
	}
}

func fetchLatestRelease(ctx context.Context, cfg CheckOptions) (release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", strings.TrimRight(cfg.APIBaseURL, "/"), cfg.Repo)
	var rel release
	if err := fetchJSON(ctx, cfg.HTTPClient, url, &rel); err != nil {
		return release{}, err
	}
	return rel, nil
}

func resolveExpectedArchiveSHA256(
	ctx context.Context,
	cfg CheckOptions,
	assets []asset,
	version string,
	archiveName string,
	archiveAsset asset,
) (string, error) {
	if digest := parseSHA256Digest(archiveAsset.Digest); digest != "" {
		return digest, nil
	}

	checksumAsset, ok := findChecksumAsset(assets, version)
	if !ok {
		return "", nil
	}

	checksumRaw, err := downloadToString(ctx, cfg.HTTPClient, checksumAsset.BrowserDownloadURL)
	if err != nil {
		return "", fmt.Errorf("download checksum file: %w", err)
	}
	checksums := parseChecksums(checksumRaw)
	if sum, ok := checksums[archiveName]; ok {
		return strings.ToLower(sum), nil
	}
	return "", fmt.Errorf("checksum for %s not found in %s", archiveName, checksumAsset.Name)
}

func findAssetByName(assets []asset, name string) (asset, bool) {
	for _, item := range assets {
		if item.Name == name {
			return item, true
		}
	}
	return asset{}, false
}

func findChecksumAsset(assets []asset, version string) (asset, bool) {
	candidates := []string{
		fmt.Sprintf("sentinel-%s-checksums.txt", version),
		"sentinel-checksums.txt",
		"checksums.txt",
	}
	for _, name := range candidates {
		if item, ok := findAssetByName(assets, name); ok {
			return item, true
		}
	}
	return asset{}, false
}

func parseSHA256Digest(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(raw), "sha256:") {
		raw = raw[len("sha256:"):]
	}
	raw = strings.TrimSpace(strings.ToLower(raw))
	if len(raw) != 64 {
		return ""
	}
	for _, r := range raw {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return ""
		}
	}
	return raw
}

func parseChecksums(raw string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		hash := strings.ToLower(strings.TrimSpace(fields[0]))
		name := strings.TrimLeft(strings.TrimSpace(fields[len(fields)-1]), "*")
		if len(hash) != 64 || name == "" {
			continue
		}
		out[name] = hash
	}
	return out
}

func fetchJSON(ctx context.Context, client *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "sentinel-updater")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("unexpected response %d from %s: %s", resp.StatusCode, url, strings.TrimSpace(string(body)))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func downloadToString(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "sentinel-updater")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("unexpected response %d from %s: %s", resp.StatusCode, url, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func downloadToFile(ctx context.Context, client *http.Client, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "sentinel-updater")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("unexpected response %d from %s: %s", resp.StatusCode, url, strings.TrimSpace(string(body)))
	}

	file, err := os.Create(path) //nolint:gosec // path is managed by internal updater flow.
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return err
	}
	return nil
}

func extractBinaryFromArchive(archivePath, outputPath string) error {
	in, err := os.Open(archivePath) //nolint:gosec // archive path comes from internal temporary directory.
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	gz, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		name := filepath.Base(strings.TrimSpace(header.Name))
		if name != binaryName {
			continue
		}
		if header.Size <= 0 {
			return errors.New("archive contains invalid sentinel binary size")
		}
		if header.Size > maxExtractedBinSize {
			return fmt.Errorf("archive sentinel binary exceeds max size (%d bytes)", maxExtractedBinSize)
		}

		out, err := os.OpenFile(outputPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700) //nolint:gosec // output path is internal updater path.
		if err != nil {
			return err
		}

		limited := io.LimitReader(tr, header.Size)
		written, copyErr := io.Copy(out, limited)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if written != header.Size {
			return errors.New("archive sentinel binary size mismatch")
		}
		return nil
	}

	return errors.New("archive does not contain sentinel binary")
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // path is controlled by updater internals.
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func resolveExecPath(raw string) (string, error) {
	path := strings.TrimSpace(raw)
	if path == "" {
		current, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("resolve executable path: %w", err)
		}
		path = current
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if strings.Contains(path, "\n") || strings.Contains(path, "\r") {
		return "", errors.New("invalid executable path")
	}
	return path, nil
}

func installBinary(execPath, newPath, backupPath string) error {
	if _, err := os.Stat(execPath); err != nil {
		return fmt.Errorf("stat current binary: %w", err)
	}

	if _, err := os.Stat(backupPath); err == nil {
		if removeErr := os.Remove(backupPath); removeErr != nil {
			return fmt.Errorf("remove previous backup: %w", removeErr)
		}
	}

	if err := os.Rename(execPath, backupPath); err != nil {
		return fmt.Errorf("backup current binary: %w", err)
	}
	if err := os.Rename(newPath, execPath); err != nil {
		_ = os.Rename(backupPath, execPath)
		return fmt.Errorf("place new binary: %w", err)
	}
	if err := os.Chmod(execPath, 0o755); err != nil { //nolint:gosec // installed binary must be executable.
		return fmt.Errorf("chmod installed binary: %w", err)
	}
	return nil
}

func rollbackBinary(execPath, backupPath string) error {
	_ = os.Remove(execPath)
	if err := os.Rename(backupPath, execPath); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}
	return nil
}

func runCommand(ctx context.Context, name string, args ...string) error {
	cmd := execCommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, msg)
	}
	return nil
}

var execCommandContext = func(ctx context.Context, name string, args ...string) command {
	return osCommand{ctx: ctx, name: name, args: args}
}

type command interface {
	CombinedOutput() ([]byte, error)
}

type osCommand struct {
	ctx  context.Context
	name string
	args []string
}

func (c osCommand) CombinedOutput() ([]byte, error) {
	cmd := execCommandFactory(c.ctx, c.name, c.args...)
	return cmd.CombinedOutput()
}

var execCommandFactory = exec.CommandContext

func normalizeVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "v")
	return raw
}

type semver struct {
	major      int
	minor      int
	patch      int
	prerelease string
	valid      bool
}

func parseSemver(raw string) semver {
	raw = normalizeVersion(raw)
	if raw == "" {
		return semver{}
	}
	parts := strings.SplitN(raw, "-", 2)
	core := strings.Split(parts[0], ".")
	if len(core) != 3 {
		return semver{}
	}
	major, err := strconv.Atoi(core[0])
	if err != nil {
		return semver{}
	}
	minor, err := strconv.Atoi(core[1])
	if err != nil {
		return semver{}
	}
	patch, err := strconv.Atoi(core[2])
	if err != nil {
		return semver{}
	}
	v := semver{major: major, minor: minor, patch: patch, valid: true}
	if len(parts) == 2 {
		v.prerelease = parts[1]
	}
	return v
}

func compareVersions(leftRaw, rightRaw string) int {
	left := parseSemver(leftRaw)
	right := parseSemver(rightRaw)
	if !left.valid || !right.valid {
		if normalizeVersion(leftRaw) == normalizeVersion(rightRaw) {
			return 0
		}
		if normalizeVersion(leftRaw) == "" {
			return -1
		}
		if normalizeVersion(rightRaw) == "" {
			return 1
		}
		if normalizeVersion(leftRaw) < normalizeVersion(rightRaw) {
			return -1
		}
		return 1
	}
	if left.major != right.major {
		if left.major < right.major {
			return -1
		}
		return 1
	}
	if left.minor != right.minor {
		if left.minor < right.minor {
			return -1
		}
		return 1
	}
	if left.patch != right.patch {
		if left.patch < right.patch {
			return -1
		}
		return 1
	}
	return comparePrerelease(left.prerelease, right.prerelease)
}

func isCurrentUpToDate(current, latest string) bool {
	current = normalizeVersion(current)
	latest = normalizeVersion(latest)
	if current == "" || latest == "" {
		return false
	}
	currentSemver := parseSemver(current)
	latestSemver := parseSemver(latest)
	if !currentSemver.valid || !latestSemver.valid {
		return current == latest
	}
	return compareVersions(current, latest) >= 0
}

func comparePrerelease(left, right string) int {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == right {
		return 0
	}
	if left == "" {
		return 1
	}
	if right == "" {
		return -1
	}

	la := strings.Split(left, ".")
	rb := strings.Split(right, ".")
	limit := len(la)
	if len(rb) > limit {
		limit = len(rb)
	}
	for i := 0; i < limit; i++ {
		if i >= len(la) {
			return -1
		}
		if i >= len(rb) {
			return 1
		}
		a := la[i]
		b := rb[i]
		if a == b {
			continue
		}
		aNum, aErr := strconv.Atoi(a)
		bNum, bErr := strconv.Atoi(b)
		switch {
		case aErr == nil && bErr == nil:
			if aNum < bNum {
				return -1
			}
			return 1
		case aErr == nil:
			return -1
		case bErr == nil:
			return 1
		default:
			if a < b {
				return -1
			}
			return 1
		}
	}
	return 0
}

func withUpdateLock(dataDir string, fn func() error) error {
	if strings.TrimSpace(dataDir) == "" {
		return fn()
	}
	lockPath := filepath.Join(dataDir, "updater", "update.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return fmt.Errorf("create updater lock directory: %w", err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // lock path is controlled by updater internals.
	if err != nil {
		return fmt.Errorf("open updater lock file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.New("another sentinel update is already running")
	}
	defer func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	}()

	return fn()
}

func statePath(dataDir string) string {
	return filepath.Join(dataDir, "updater", "state.json")
}

func loadState(dataDir string) (State, error) {
	path := statePath(dataDir)
	data, err := os.ReadFile(path) //nolint:gosec // internal state file path
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, err
	}
	return st, nil
}

func writeState(dataDir string, mutate func(*State)) {
	if strings.TrimSpace(dataDir) == "" {
		return
	}
	st, _ := loadState(dataDir)
	mutate(&st)
	stPath := statePath(dataDir)
	if err := os.MkdirAll(filepath.Dir(stPath), 0o700); err != nil {
		return
	}
	payload, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	tmpPath := stPath + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0o600); err != nil { //nolint:gosec // internal state write
		return
	}
	_ = os.Rename(tmpPath, stPath)
}

func recordCheckState(dataDir string, result CheckResult) {
	writeState(dataDir, func(st *State) {
		st.LastCheckedAt = result.CheckedAt
		st.CurrentVersion = result.CurrentVersion
		st.LatestVersion = result.LatestVersion
		st.UpToDate = result.UpToDate
		st.LastReleaseURL = result.ReleaseURL
		st.LastExpectedSHA256 = result.ExpectedSHA256
		st.LastError = ""
	})
}

func recordApplyState(dataDir string, check CheckResult, result ApplyResult, errText string) {
	writeState(dataDir, func(st *State) {
		st.LastCheckedAt = check.CheckedAt
		st.LastAppliedAt = result.AppliedAt
		st.CurrentVersion = result.LatestVersion
		st.LatestVersion = check.LatestVersion
		st.UpToDate = true
		st.LastReleaseURL = check.ReleaseURL
		st.LastAppliedBinary = result.BinaryPath
		st.LastAppliedBackup = result.BackupPath
		st.LastExpectedSHA256 = check.ExpectedSHA256
		st.LastError = strings.TrimSpace(errText)
	})
}

func recordStateError(dataDir string, now time.Time, currentVersion string, err error) {
	if err == nil {
		return
	}
	writeState(dataDir, func(st *State) {
		st.LastCheckedAt = now
		if strings.TrimSpace(currentVersion) != "" {
			st.CurrentVersion = normalizeVersion(currentVersion)
		}
		st.LastError = err.Error()
	})
}
