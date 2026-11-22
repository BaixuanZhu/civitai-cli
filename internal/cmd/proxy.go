package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"civitai-cli/internal/app"
	"civitai-cli/internal/config"
	"civitai-cli/internal/proxy"
	"civitai-cli/internal/report"
)

// Proxy runs the proxy command: civ proxy [set|get|unset]. It manages the
// persisted outbound proxy URL: set validates before saving, get reports
// the persisted value, the effective value, and the source, and unset
// clears the proxy while preserving the token. Bare `civ proxy` prints the
// same status as get. This is a purely local operation (no network); the
// proxy takes effect on the data commands through the client factories.
// Exit codes: 0 success, 1 usage error (unknown action, bad proxy URL, or
// wrong argument count).
func Proxy(args []string) {
	execute("proxy", args, nil, runProxy)
}

// runProxy is the pure proxy implementation (testable without os.Exit):
// it dispatches on the first positional action and validates argument
// counts. Every error it returns is usage-class, so the shared exit-code
// map sends it to exit 1.
func runProxy(ctx context.Context, g *globals, pos []string) error {
	if len(pos) == 0 {
		return reportProxyStatus(g) // bare `civ proxy` = friendly status
	}
	action, rest := pos[0], pos[1:]
	switch action {
	case "set":
		return runProxySet(g, rest)
	case "get":
		if len(rest) != 0 {
			return errors.New("proxy get takes no arguments")
		}
		return reportProxyStatus(g)
	case "unset":
		return runProxyUnset(g, rest)
	default:
		return fmt.Errorf("unknown proxy action %q; want set, get, or unset", action)
	}
}

// runProxySet implements `civ proxy set <url>`: the URL is validated with
// proxy.Validate BEFORE any write, so malformed input is a usage error that
// leaves the config untouched (prd R5/AC4). On success the loaded config is
// modified and saved so the sibling token field survives (load-modify-save,
// never a fresh Config).
func runProxySet(g *globals, pos []string) error {
	if len(pos) != 1 {
		return errors.New("proxy set requires exactly one argument: the proxy URL (e.g. http://127.0.0.1:7890)")
	}
	raw := strings.TrimSpace(pos[0])
	if _, err := proxy.Validate(raw); err != nil {
		return err
	}
	cfg, _ := config.Load()
	cfg.Proxy = raw
	if err := config.Save(cfg); err != nil {
		return err
	}
	if g.JSON {
		return report.WriteIndent(g.stdout(), proxyResultJSON{Status: "set", Proxy: raw, Config: config.Path()})
	}
	fmt.Fprintf(g.stdout(), "Proxy set to %s. Saved to %s\n", raw, config.Path())
	return nil
}

// runProxyUnset implements `civ proxy unset`: load-modify-save with Proxy
// zeroed. The file is rewritten, never deleted, so the token survives.
// Clearing an already-empty proxy is a friendly no-op (idempotent, exit 0).
func runProxyUnset(g *globals, pos []string) error {
	if len(pos) != 0 {
		return errors.New("proxy unset takes no arguments")
	}
	cfg, _ := config.Load()
	cfg.Proxy = ""
	if err := config.Save(cfg); err != nil {
		return err
	}
	if g.JSON {
		return report.WriteIndent(g.stdout(), proxyResultJSON{Status: "unset", Config: config.Path()})
	}
	fmt.Fprintf(g.stdout(), "Proxy unset. Saved to %s\n", config.Path())
	return nil
}

// proxyStatusJSON is the --json payload of `civ proxy get` and bare
// `civ proxy` (prd R1): the persisted config value, the effective explicit
// proxy, and where the effective value comes from.
type proxyStatusJSON struct {
	Persisted string `json:"persisted"`
	Effective string `json:"effective"`
	Source    string `json:"source"`
}

// proxyResultJSON is the --json payload of `civ proxy set` and
// `civ proxy unset`: what changed and where it was written.
type proxyResultJSON struct {
	Status string `json:"status"` // "set" or "unset"
	Proxy  string `json:"proxy,omitempty"`
	Config string `json:"config"`
}

// reportProxyStatus implements `civ proxy get` and the bare `civ proxy`
// friendly default. The effective value is the explicit chain only
// (config.ResolveProxy("") — the --proxy flag is per-invocation and applies
// to data commands, not to this report); an empty effective value means
// HTTP(S)_PROXY (env) or a direct connection applies, which the source
// field makes observable. With --verbose a debug line names the effective
// proxy and its source on stderr (prd R6).
func reportProxyStatus(g *globals) error {
	cfg, _ := config.Load()
	effective := config.ResolveProxy("")
	source := proxySource(cfg.Proxy)
	if effective != "" {
		app.Debugf("proxy: routing through %s (source: %s)", effective, source)
	} else {
		app.Debugf("proxy: no explicit proxy configured (source: %s)", source)
	}
	if g.JSON {
		return report.WriteIndent(g.stdout(), proxyStatusJSON{
			Persisted: cfg.Proxy,
			Effective: effective,
			Source:    source,
		})
	}
	fmt.Fprintf(g.stdout(), "Persisted proxy: %s\n", proxyDisplay(cfg.Proxy))
	fmt.Fprintf(g.stdout(), "Effective proxy: %s\n", proxyDisplay(effective))
	fmt.Fprintf(g.stdout(), "Source: %s\n", source)
	return nil
}

// proxySource reports where the effective proxy comes from: "config" when
// a persisted proxy URL exists, "env" when HTTPS_PROXY/HTTP_PROXY is set
// (the DefaultTransport fallback), else "direct". Detection stays simple
// and stdlib-only, folding the two standard variable names (design §2);
// the effective value itself is resolved by config.ResolveProxy.
func proxySource(persisted string) string {
	if strings.TrimSpace(persisted) != "" {
		return "config"
	}
	for _, name := range [...]string{"HTTPS_PROXY", "HTTP_PROXY"} {
		if os.Getenv(name) != "" {
			return "env"
		}
	}
	return "direct"
}

// proxyDisplay renders a proxy value for the human report, mapping an
// empty value to "(none)".
func proxyDisplay(v string) string {
	if strings.TrimSpace(v) == "" {
		return "(none)"
	}
	return v
}
