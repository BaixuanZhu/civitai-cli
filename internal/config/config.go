// Package config persists the civ API token and resolves the token
// priority chain. The config file lives at os.UserConfigDir()/civ/config.json
// (%AppData%\civ on Windows) and holds {"token":"..."}; writes use 0600
// semantics (best-effort on Windows). It is a leaf package: the cmd layer
// calls ResolveToken at the M1 token-resolution seam and login/logout call
// Save/Delete. Token priority is --token flag > CIVITAI_API_KEY > config file
// (parent design D8).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"civitai-cli/internal/app"
)

// Config is the persisted civ configuration. Only the API token is stored
// in v1; the struct leaves room for future keys without a schema break.
type Config struct {
	Token string `json:"token"`
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
