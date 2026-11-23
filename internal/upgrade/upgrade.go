// Package upgrade checks GitHub Releases and updates civ itself (invoked by
// the `civ upgrade` subcommand).
//
// Mechanism: call the GitHub API /repos/{owner}/{repo}/releases/latest for the
// latest release, compare tag_name against the current version, download the
// agreed asset (zip containing a single exe), verify SHA256 (checksums.txt),
// then extract and replace the current exe.
// Windows cannot overwrite a running exe but can rename it: old exe -> .bak,
// new exe moved into place; if the .bak cannot be deleted because the old
// process still holds it, CleanupStaleBak removes it on the next startup.
//
// Deployment notes (when publishing a release):
//   - tags use the v0.1.0 format
//   - upload asset: civitai-cli-windows-amd64.zip (zip contains a single civ.exe)
//   - upload checksums.txt (sha256sum format, shared by upgrade and install.ps1)
package upgrade

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"civitai-cli/internal/app"
	"civitai-cli/internal/config"
	"civitai-cli/internal/proxy"
)

// githubRepo is the GitHub repository publishing civitai-cli (owner/repo format).
const githubRepo = "BaixuanZhu/civitai-cli"

// exeName is the agreed executable file name inside the zip (matches BINARY in the Makefile).
const exeName = "civ.exe"

// downloadClient is the single HTTP client for all upgrade traffic — the
// GitHub API release check (fetchLatestRelease) and the release asset
// downloads — without an overall timeout: timeout control is left to the
// transfer itself so slow networks don't kill large downloads. Its Transport
// starts each run as nil (environment proxies apply via http.DefaultTransport)
// unless applyProxy installs a flag- or github-slot-derived transport.
var downloadClient = &http.Client{Timeout: 0}

// githubRelease mirrors the GitHub API releases/latest response (only the needed fields).
type githubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

// expectedAssetName returns the release asset file name agreed for this platform.
// Full name = civitai-cli-{GOOS}-{GOARCH}.zip, e.g. civitai-cli-windows-amd64.zip.
func expectedAssetName() string {
	return fmt.Sprintf("civitai-cli-%s-%s.zip", runtime.GOOS, runtime.GOARCH)
}

// applyProxy parses the invocation's own args (--proxy) and installs the
// resolved proxy as downloadClient's Transport, so the GitHub API check and
// the asset download (unified onto downloadClient) share one route.
// Precedence: --proxy flag > saved github slot (config.ResolveGithubProxy) >
// environment via nil Transport (http.DefaultTransport) > direct; the civ
// proxy slot is never read here — shared proxy IPs are what GitHub
// rate-limits with 403 (task 11-23-upgrade-direct design D1).
//
// An invalid --proxy flag value is a usage error: it is returned so Run exits
// 1 before any network access. An invalid hand-edited github slot instead
// degrades — a [WARN] line on stderr and the nil Transport kept — mirroring
// config's corrupt-file philosophy: a bad saved value must never fail an
// upgrade (design D3). The Transport is reset to nil before resolution so
// every run starts from env/direct semantics unless a flag/slot transport
// installs below.
func applyProxy(args []string) error {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	proxyFlag := fs.String("proxy", "", "relay GitHub traffic through this proxy for this invocation")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("invalid usage: %w (try \"civ upgrade help\")", err)
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q (usage: civ upgrade [--proxy <url>])", fs.Arg(0))
	}
	if v := *proxyFlag; v != "" {
		if _, err := proxy.Validate(v); err != nil {
			return fmt.Errorf("invalid --proxy value: %w", err)
		}
	}

	downloadClient.Transport = nil
	resolved := config.ResolveGithubProxy(*proxyFlag)
	if resolved == "" {
		return nil // no explicit proxy: env proxies via http.DefaultTransport
	}
	tr, err := proxy.NewTransport(resolved)
	if err != nil {
		// Only reachable for a hand-edited github slot: the flag value was
		// validated above, so degrade instead of failing the upgrade.
		app.Warnf("invalid github proxy %q (%v); falling back to environment/direct", resolved, err)
		return nil
	}
	downloadClient.Transport = tr
	return nil
}

// Help returns the curated `civ upgrade help` page as a string. main prints
// it verbatim for `civ upgrade help` / -h / --help and returns exit 0; Run()
// behavior (including --proxy) is unchanged.
func Help() string {
	return `civ upgrade - Self-update from the latest GitHub Release

Usage:
  civ upgrade [--proxy <url>]

What it does:
  Checks the latest GitHub Release, compares it with the running version
  (a local build newer than the release is never downgraded), downloads
  the platform zip asset, verifies its SHA256 against the release's
  checksums.txt (releases without one skip the check with a warning),
  then extracts civ.exe and swaps it in. Windows cannot overwrite a
  running exe, so the old exe is first renamed to civ.exe.bak; the .bak
  is deleted immediately on success, or on the next startup if the old
  process still holds it.

Options:
  --proxy <url>  relay the GitHub API check and asset download through
    this proxy for this invocation only (values: http, https, socks5,
    socks5h; default: none — direct connection). An invalid value is a
    usage error (exit 1) before any network access. Wins over the github
    proxy slot.

Examples:
  civ upgrade
  civ upgrade --proxy http://127.0.0.1:7890
  civ upgrade -h

Exit codes:
  0 success or already up to date   1 check, download, verify, or replace failure
  (a usage error, e.g. an invalid --proxy value, also exits 1 before any
  network access)

Output:
  Human-readable progress lines on stdout; --json does not apply (upgrade
  bypasses the global flag runner, so there is no machine-output form).

Proxy:
  Connects to GitHub directly by default. The civ proxy slot
  (civ proxy set) is intentionally NOT consulted — GitHub rate-limits
  shared proxy IPs with 403. With no --proxy flag, the github proxy slot
  (civ proxy set --github <url>) is honored; otherwise
  HTTP(S)_PROXY/NO_PROXY environment variables apply. An invalid github
  slot value warns and degrades to env/direct, never failing the upgrade.
`
}

// Run checks for and installs the latest version. args is everything after
// "civ upgrade" (main has already routed bare help tokens to Help); Run parses
// its own --proxy flag via applyProxy. A usage error — unknown flag, extra
// argument, or an invalid --proxy value — fails via app.Fail (stderr + exit
// 1) before any network access; on success or "already current" it returns
// normally so main can exit 0.
func Run(args []string) {
	if err := applyProxy(args); err != nil {
		app.Fail(err)
	}

	fmt.Printf("Checking the latest release of %s...\n", githubRepo)
	rel, err := fetchLatestRelease()
	if err != nil {
		app.Fail(fmt.Errorf("failed to check for updates: %w", err))
	}

	latestVersion := strings.TrimPrefix(rel.TagName, "v")
	fmt.Printf("  Latest: %s (current: %s)\n", latestVersion, app.Version)

	// Semantic comparison against the local version prevents a dev/patched
	// build from being silently downgraded by an older release.
	switch compareVersions(app.Version, latestVersion) {
	case 0:
		fmt.Println("Already up to date.")
		return
	case 1:
		fmt.Println("Local version is newer than the latest release, skipping update.")
		return
	}

	// Find the asset matching the current platform.
	assetURL := ""
	want := expectedAssetName()
	for _, a := range rel.Assets {
		if a.Name == want {
			assetURL = a.BrowserDownloadURL
			break
		}
	}
	if assetURL == "" {
		app.Fail(fmt.Errorf("asset %s not found in the latest release.\n   Download it manually from %s.", want, rel.HTMLURL))
	}

	fmt.Printf("Downloading %s ...\n", want)
	tmpZip := filepath.Join(os.TempDir(), "civ-upgrade-"+latestVersion+".zip")
	if err := downloadFile(assetURL, tmpZip, 3); err != nil {
		app.Fail(fmt.Errorf("download failed: %w", err))
	}
	defer os.Remove(tmpZip)

	// SHA256 verification: take the expected hash from the release's checksums.txt
	// asset and compare it against the downloaded file. If an old release has no
	// checksums.txt, print a warning and continue (backwards compatible).
	if expectedHash, ok := fetchExpectedChecksum(rel, want); ok {
		fmt.Print("Verifying SHA256... ")
		got, err := fileSHA256(tmpZip)
		if err != nil {
			app.Fail(fmt.Errorf("failed to compute SHA256 of the download: %w", err))
		}
		if got != expectedHash {
			os.Remove(tmpZip)
			app.Fail(fmt.Errorf("checksum mismatch\n   want: %s\n   got:  %s", expectedHash, got))
		}
		fmt.Println("OK")
	} else {
		fmt.Println("Release has no checksums.txt, skipping integrity check.")
	}

	fmt.Print("Extracting... ")
	// Extract into the current exe's directory to avoid a cross-volume rename
	// failure (os.Rename uses MoveFileEx, which cannot cross volumes).
	selfExe, err := os.Executable()
	if err != nil {
		app.Fail(fmt.Errorf("failed to locate the current exe: %w", err))
	}
	selfDir := filepath.Dir(selfExe)
	tmpExe, err := extractSingleExe(tmpZip, selfDir)
	if err != nil {
		app.Fail(fmt.Errorf("extract failed: %w", err))
	}
	defer os.Remove(tmpExe)
	fmt.Println("done")

	fmt.Print("Replacing binary... ")
	if err := replaceSelf(tmpExe); err != nil {
		app.Fail(fmt.Errorf("replace failed: %w", err))
	}
	fmt.Println("done")

	fmt.Printf("\nUpdated to %s\n", latestVersion)
	fmt.Println("   The new version takes effect on next launch.")
}

// fetchLatestRelease calls the GitHub API for the latest release. It rides
// downloadClient (not http.DefaultClient) so the run's resolved proxy applies
// to this call too and upgrade traffic stays on one client.
func fetchLatestRelease() (*githubRelease, error) {
	u := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", app.UserAgent())
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := downloadClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var rel githubRelease
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("failed to parse the GitHub response: %w", err)
	}
	return &rel, nil
}

// downloadFile downloads url to dest, retrying up to retries times on failure (linear backoff).
func downloadFile(url, dest string, retries int) error {
	var lastErr error
	for i := 0; i <= retries; i++ {
		if i > 0 {
			time.Sleep(time.Duration(i) * time.Second)
		}
		lastErr = downloadOnce(url, dest)
		if lastErr == nil {
			return nil
		}
	}
	return lastErr
}

// downloadOnce is a single download attempt: streamed to disk, non-200 counts as failure.
func downloadOnce(url, dest string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", app.UserAgent())

	resp, err := downloadClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// extractSingleExe extracts exeName from the zip into a temp file in dir and
// returns its path. Placing the temp file in dir (usually the current exe's
// directory) guarantees the later os.Rename stays on the same volume —
// Windows MoveFileEx degrades to copy+delete across volumes, which fails for
// a running exe.
func extractSingleExe(zipPath, dir string) (string, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	for _, f := range reader.File {
		if filepath.Base(f.Name) != exeName {
			continue
		}
		out, err := os.CreateTemp(dir, "civ-new-*.exe")
		if err != nil {
			return "", err
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			os.Remove(out.Name())
			return "", err
		}
		if _, err := io.Copy(out, rc); err != nil {
			rc.Close()
			out.Close()
			os.Remove(out.Name())
			return "", err
		}
		rc.Close()
		out.Close()
		return out.Name(), nil
	}
	return "", fmt.Errorf("zip does not contain %s", exeName)
}

// replaceSelf replaces the currently running civ with the new exe.
// The standard Windows approach: rename the old exe to .bak (a running exe
// cannot be overwritten but can be renamed), then move the new exe into place.
func replaceSelf(newExePath string) error {
	currentExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to locate the current exe: %w", err)
	}
	currentExe, _ = filepath.Abs(currentExe)
	return replaceExeAt(currentExe, newExePath)
}

// replaceExeAt atomically replaces currentExe with newExe (via a .bak hop).
// Split out with explicit paths so table-driven tests can use fake exes in a
// temp directory.
func replaceExeAt(currentExe, newExePath string) error {
	bakPath := currentExe + ".bak"
	os.Remove(bakPath) // clean up a possibly stale .bak

	// 1. Rename the current exe to .bak
	if err := os.Rename(currentExe, bakPath); err != nil {
		return fmt.Errorf("failed to rename the old exe (file may be in use): %w", err)
	}
	// 2. Move the new exe into place
	if err := os.Rename(newExePath, currentExe); err != nil {
		os.Rename(bakPath, currentExe) // roll back
		return fmt.Errorf("failed to move the new exe: %w", err)
	}
	// 3. Try to delete the .bak (may fail while the old process holds it; ignored)
	os.Remove(bakPath)
	return nil
}

// fetchExpectedChecksum parses the expected SHA256 of filename from the
// release's checksums.txt asset. checksums.txt uses the GNU coreutils
// sha256sum format: "<hash>  <filename>".
// Returns ("", false) when the checksums.txt asset is missing or the file
// name does not match.
func fetchExpectedChecksum(rel *githubRelease, filename string) (string, bool) {
	var checksumURL string
	for _, a := range rel.Assets {
		if a.Name == "checksums.txt" {
			checksumURL = a.BrowserDownloadURL
			break
		}
	}
	if checksumURL == "" {
		return "", false
	}

	tmpChecksum := filepath.Join(os.TempDir(), "civ-checksums.txt")
	if err := downloadFile(checksumURL, tmpChecksum, 3); err != nil {
		return "", false
	}
	defer os.Remove(tmpChecksum)

	data, err := os.ReadFile(tmpChecksum)
	if err != nil {
		return "", false
	}
	return parseChecksum(string(data), filename)
}

// parseChecksum finds the hash for filename in sha256sum-format text.
// GNU coreutils has two formats:
//
//	"<hash>  <filename>"  text mode (two spaces, no asterisk)
//	"<hash> *<filename>"  binary mode (one space + leading asterisk)
//
// The sha256sum bundled with Windows Git Bash defaults to binary mode (a
// leading '*' on the name), so strip a leading '*' before comparing.
// Pure function, for table-driven tests.
func parseChecksum(text, filename string) (string, bool) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// split into hash and name
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		hash, name := fields[0], fields[1]
		name = strings.TrimPrefix(name, "*") // tolerate binary-mode '*' prefix
		if name == filename {
			return hash, true
		}
	}
	return "", false
}

// compareVersions compares two version numbers semantically, returning
// -1/0/1 (a<b / a==b / a>b). Inputs should be clean version numbers (no v
// prefix), e.g. "0.2.0"; a non-numeric suffix is allowed (dev builds like
// "0.1.0-dev", suffix segments count as 0). Splits on non-digit characters
// and compares segment by segment; missing segments count as 0.
// Pure function, for table-driven tests.
func compareVersions(a, b string) int {
	pa := versionSegments(a)
	pb := versionSegments(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		va, vb := 0, 0
		if i < len(pa) {
			va = pa[i]
		}
		if i < len(pb) {
			vb = pb[i]
		}
		if va != vb {
			if va < vb {
				return -1
			}
			return 1
		}
	}
	return 0
}

// versionSegments splits a version number into digit segments on non-digit
// characters, e.g. "0.2.1" -> [0,2,1]. Invalid segments count as 0. Pure function.
func versionSegments(s string) []int {
	var parts []int
	for _, seg := range strings.FieldsFunc(s, func(r rune) bool {
		return r < '0' || r > '9'
	}) {
		if seg == "" {
			continue
		}
		n, err := strconv.Atoi(seg)
		if err != nil {
			n = 0
		}
		parts = append(parts, n)
	}
	return parts
}

// fileSHA256 computes the SHA256 of a file (lowercase hex).
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CleanupStaleBak removes a leftover civ.exe.bak.
//
// replaceSelf renames the old exe to .bak during an upgrade and deletes it
// immediately on success; but while the old exe is still running the delete
// fails (Windows file lock), so the .bak can linger. This is called on every
// startup to clear .baks left by the last upgrade. Failures are silently
// ignored (a still-locked .bak or missing permissions must not block the main
// flow).
func CleanupStaleBak() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	cleanupBakAt(exePath)
}

// cleanupBakAt removes the .bak belonging to exePath. Pure path manipulation,
// for table-driven tests. Returns whether a file was actually removed
// (a non-existing .bak counts as nothing to clean).
func cleanupBakAt(exePath string) bool {
	bakPath := exePath + ".bak"
	if _, err := os.Stat(bakPath); err != nil {
		return false // does not exist, nothing to clean
	}
	return os.Remove(bakPath) == nil
}
