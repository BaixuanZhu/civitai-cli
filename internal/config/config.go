// Package config persists the civ API token and the two scoped proxy URLs
// and resolves their priority chains. The config file lives at
// os.UserConfigDir()/civ/config.json (%AppData%\civ on Windows) and holds
// {"token":"...","proxy":"...","githubProxy":"..."}; writes use 0600
// semantics (best-effort on Windows). It stays at the leaf level (imports
// only the app and proxy leaves): the cmd layer calls ResolveToken at the
// M1 token-resolution seam and ResolveProxy for outbound proxying;
// login/logout call Save/Delete. Token priority is --token flag >
// CIVITAI_API_KEY > config file (parent design D8); each proxy scope
// resolves --proxy flag > its own saved slot, with environment proxies
// applying only when neither is set (task 11-22 design D2). The githubProxy
// slot is consumed only by civ upgrade; data commands never read it
// (task 11-23-github-proxy-slot).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"civitai-cli/internal/app"
	"civitai-cli/internal/proxy"
)

// Config is the persisted civ configuration. It stores the API token and,
// since proxy support, the optional outbound proxy slots: Proxy for
// civitai (data-command) traffic and GithubProxy for GitHub (upgrade)
// traffic, so each scope can relay through a different proxy. The schema
// is additive: files from earlier versions decode with empty proxy slots
// and need no migration (task 11-22 design §2; the github slot follows the
// same tolerant-decode contract in task 11-23-github-proxy-slot).
type Config struct {
	Token       string `json:"token"`
	Proxy       string `json:"proxy"`
	GithubProxy string `json:"githubProxy,omitempty"`
}

// Path returns the config file location, os.UserConfigDir()/civ/config.json.
// It returns "" when the user config directory cannot be determined (no
// AppData/XDG_CONFIG_HOME/HOME); callers treat "" as "no config available".
func Path() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "civ", "config.json")
}

// Load reads the config file. A missing file (or an undeterminable path)
// yields the zero Config and a nil error — anonymous operation is the
// default, not a failure. A corrupt or unreadable file warns on stderr and
// degrades to the zero Config so a broken config never blocks read commands.
func Load() (Config, error) {
	p := Path()
	if p == "" {
		return Config{}, nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		app.Warnf("config unreadable (%v); continuing without a saved token", err)
		return Config{}, nil
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		app.Warnf("config corrupt (%v); continuing without a saved token", err)
		return Config{}, nil
	}
	return c, nil
}

// Save writes c to the config file, creating the parent directory with
// 0700 and the file with 0600 (owner-only; best-effort ACL on Windows).
// The write is atomic (temp file + rename) so a crash never leaves a
// half-written token file. It returns an error when the path is unknown
// or the write fails.
func Save(c Config) error {
	p := Path()
	if p == "" {
		return fmt.Errorf("config: cannot determine user config directory")
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("config: create %s: %w", dir, err)
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("config: encode: %w", err)
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(dir, "config-*.json.tmp")
	if err != nil {
		return fmt.Errorf("config: create temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("config: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("config: close temp: %w", err)
	}
	// CreateTemp makes the file 0600 already; chmod best-effort for clarity.
	if err := os.Chmod(tmpName, 0o600); err != nil {
		app.Warnf("config: chmod 0600 best-effort failed: %v", err)
	}
	if err := os.Rename(tmpName, p); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("config: rename: %w", err)
	}
	return nil
}

// Delete removes the config file (logout). It is idempotent: a missing
// file or an undeterminable path yields nil. Other removal errors are
// returned wrapped.
func Delete() error {
	p := Path()
	if p == "" {
		return nil
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("config: delete %s: %w", p, err)
	}
	return nil
}

// ResolveToken implements the token priority chain: a non-empty flagToken
// wins, then CIVITAI_API_KEY, then the saved config token. Load failures
// degrade to anonymous (empty) rather than erroring, so token resolution
// never blocks a read command.
func ResolveToken(flagToken string) string {
	if flagToken != "" {
		return flagToken
	}
	if env := os.Getenv("CIVITAI_API_KEY"); env != "" {
		return env
	}
	c, _ := Load()
	return c.Token
}

// ResolveProxy implements the explicit-proxy precedence chain: a flagProxy
// that is non-empty after trimming wins, else the saved config proxy
// (delegated to proxy.Resolve). An empty result means "no explicit proxy" —
// callers keep a nil Transport so HTTP_PROXY/HTTPS_PROXY still apply via
// http.DefaultTransport. Load failures degrade to "" rather than erroring,
// mirroring ResolveToken, so proxy resolution never blocks a command
// (task 11-22 design D2).
func ResolveProxy(flagProxy string) string {
	c, _ := Load()
	return proxy.Resolve(flagProxy, c.Proxy)
}

// ResolveGithubProxy implements the GitHub-scope explicit-proxy precedence
// chain, mirroring ResolveProxy: a flagProxy that is non-empty after
// trimming wins, else the saved githubProxy slot. The result is consumed
// ONLY by civ upgrade (task 11-23-upgrade-direct); data commands never read
// the github slot, and no command reads the civitai slot for GitHub
// traffic. Load failures degrade to "" rather than erroring, mirroring
// ResolveProxy, so proxy resolution never blocks a command.
func ResolveGithubProxy(flagProxy string) string {
	c, _ := Load()
	return proxy.Resolve(flagProxy, c.GithubProxy)
}
