package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"civitai-cli/internal/app"
	"civitai-cli/internal/config"
	"civitai-cli/internal/proxy"
	"civitai-cli/internal/report"
)

// proxyOptions carries the proxy command's own flags: --github switches
// set/unset to the GitHub-scoped slot (civ upgrade traffic) instead of the
// civitai slot (data commands).
type proxyOptions struct {
	github bool
}

// register binds the proxy command flags to fs. --github is the command's
// first own flag (task 11-23-github-proxy-slot design D2); get/status stay
// flagless because the report always shows both scopes.
func (o *proxyOptions) register(fs *flag.FlagSet) {
	fs.BoolVar(&o.github, "github", false, "target the GitHub-only proxy slot (civ upgrade traffic) on set/unset")
}

// proxyHelp is the curated `civ proxy help` / `-h` page (without the shared
// global block, which writeCommandHelp appends): the action forms, the two
// scopes, the per-scope proxy precedence chains, and the --json shapes.
const proxyHelp = `save, show, or clear the persisted outbound proxies

Usage:
  civ proxy [set <url> | get | unset] [--github]

Examples:
  civ proxy set http://127.0.0.1:7890
  civ proxy set --github socks5h://127.0.0.1:1080
  civ proxy
  civ proxy unset
  civ proxy unset --github

Notes:
  There are two scopes: civitai (data commands: search, show, vet, sync,
  tags) and github (civ upgrade traffic only). set saves the civitai slot;
  set --github saves the GitHub slot. unset clears BOTH slots; unset
  --github clears only the GitHub slot. Both unset forms are idempotent
  and rewrite the config file, so the saved token survives; a malformed
  URL exits 1 before anything is written.
  Bare civ proxy prints the same report as get, per scope: the persisted
  proxy, the effective explicit proxy, and the source (config, env, or
  direct). Effective is the saved slot only — the per-invocation --proxy
  flag never shows up here; an empty value means HTTP_PROXY/HTTPS_PROXY
  (env) or a direct connection applies.
  Accepted schemes are http, https, socks5, and socks5h with a host:port.
  Data commands pick their proxy: the global --proxy flag, the saved
  civitai slot, then HTTP_PROXY/HTTPS_PROXY. upgrade picks: the global
  --proxy flag, the saved GitHub slot, then HTTP_PROXY/HTTPS_PROXY. The
  two slots never cross: no command reads the civitai slot for GitHub
  traffic or the GitHub slot for civitai traffic. --json: get prints
  {persisted, effective, source, github:{persisted, effective, source}};
  set and unset print {status, proxy, config, scope?} (proxy omitted on
  unset, scope present only when --github was used).
`

// Proxy runs the proxy command: civ proxy [set|get|unset] [--github]. It
// manages the persisted outbound proxy URLs for the two scopes: the
// civitai slot (data commands) and the GitHub slot (civ upgrade traffic).
// set validates before saving, get reports per scope the persisted value,
// the effective value, and the source, and unset clears the slots while
// preserving the token: bare unset clears BOTH slots, unset --github only
// the GitHub one. Bare `civ proxy` prints the same status as get. This is
// a purely local operation (no network); the slots take effect on the data
// commands and on civ upgrade through their own resolution seams. Exit
// codes: 0 success, 1 usage error (unknown action, bad proxy URL, wrong
// argument count, or --github on get/status).
func Proxy(args []string) {
	var o proxyOptions
	execute("proxy", proxyHelp, args, o.register, func(ctx context.Context, g *globals, pos []string) error {
		return runProxy(ctx, g, pos, &o)
	})
}

// runProxy is the pure proxy implementation (testable without os.Exit):
// it dispatches on the first positional action and validates argument
// counts. Every error it returns is usage-class, so the shared exit-code
// map sends it to exit 1.
func runProxy(ctx context.Context, g *globals, pos []string, o *proxyOptions) error {
	if len(pos) == 0 {
		if o.github {
			return errors.New("proxy --github applies only to set and unset; the status report always shows both scopes")
		}
		return reportProxyStatus(g) // bare `civ proxy` = friendly status
	}
	action, rest := pos[0], pos[1:]
	switch action {
	case "set":
		return runProxySet(g, rest, o)
	case "get":
		if len(rest) != 0 {
			return errors.New("proxy get takes no arguments")
		}
		if o.github {
			return errors.New("proxy get does not take --github; the status report always shows both scopes")
		}
		return reportProxyStatus(g)
	case "unset":
		return runProxyUnset(g, rest, o)
	default:
		return fmt.Errorf("unknown proxy action %q; want set, get, or unset", action)
	}
}

// runProxySet implements `civ proxy set <url>`: the URL is validated with
// proxy.Validate BEFORE any write, so malformed input is a usage error that
// leaves the config untouched (prd R5/AC4). Without --github it saves the
// civitai slot only; with --github it saves the GitHub slot only (prd R2) —
// in both cases the loaded config is modified and saved so the sibling
// fields (token and the other slot) survive (load-modify-save, never a
// fresh Config).
func runProxySet(g *globals, pos []string, o *proxyOptions) error {
	if len(pos) != 1 {
		return errors.New("proxy set requires exactly one argument: the proxy URL (e.g. http://127.0.0.1:7890)")
	}
	raw := strings.TrimSpace(pos[0])
	if _, err := proxy.Validate(raw); err != nil {
		return err
	}
	cfg, _ := config.Load()
	scope := ""
	label := "Proxy"
	if o.github {
		cfg.GithubProxy = raw
		scope = "github"
		label = "GitHub proxy"
	} else {
		cfg.Proxy = raw
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	if g.JSON {
		return report.WriteIndent(g.stdout(), proxyResultJSON{Status: "set", Proxy: raw, Config: config.Path(), Scope: scope})
	}
	fmt.Fprintf(g.stdout(), "%s set to %s. Saved to %s\n", label, raw, config.Path())
	return nil
}

// runProxyUnset implements `civ proxy unset` and `civ proxy unset
// --github`: load-modify-save with the targeted slots zeroed. The file is
// rewritten, never deleted, so the token survives. Bare unset clears BOTH
// slots (the "stop routing everything" case, prd R3); --github clears only
// the GitHub slot. Clearing already-empty slots is a friendly no-op
// (idempotent, exit 0).
func runProxyUnset(g *globals, pos []string, o *proxyOptions) error {
	if len(pos) != 0 {
		return errors.New("proxy unset takes no arguments")
	}
	cfg, _ := config.Load()
	scope := ""
	label := "Proxy unset (both civitai and GitHub slots)."
	if o.github {
		cfg.GithubProxy = ""
		scope = "github"
		label = "GitHub proxy unset."
	} else {
		cfg.Proxy = ""
		cfg.GithubProxy = ""
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	if g.JSON {
		return report.WriteIndent(g.stdout(), proxyResultJSON{Status: "unset", Config: config.Path(), Scope: scope})
	}
	fmt.Fprintf(g.stdout(), "%s Saved to %s\n", label, config.Path())
	return nil
}

// proxyScopeStatus is the per-scope proxy status payload: the persisted
// config value, the effective explicit proxy, and where the effective value
// comes from.
type proxyScopeStatus struct {
	Persisted string `json:"persisted"`
	Effective string `json:"effective"`
	Source    string `json:"source"`
}

// proxyStatusJSON is the --json payload of `civ proxy get` and bare
// `civ proxy` (prd R1/R5): the top-level fields carry the civitai scope
// exactly as before task 11-23-github-proxy-slot; github is the additive
// nested scope object, so old consumers of the top-level keys are
// unaffected.
type proxyStatusJSON struct {
	Persisted string           `json:"persisted"`
	Effective string           `json:"effective"`
	Source    string           `json:"source"`
	Github    proxyScopeStatus `json:"github"`
}

// proxyResultJSON is the --json payload of `civ proxy set` and
// `civ proxy unset`: what changed and where it was written. Scope is
// "github" only when --github targeted that slot; it is omitted for the
// civitai scope so the pre-github result shape stays byte-compatible
// (prd R5).
type proxyResultJSON struct {
	Status string `json:"status"` // "set" or "unset"
	Proxy  string `json:"proxy,omitempty"`
	Config string `json:"config"`
	Scope  string `json:"scope,omitempty"`
}

// reportProxyStatus implements `civ proxy get` and the bare `civ proxy`
// friendly default. The civitai block keeps its three historical lines
// byte-identical (integration assertions depend on them); the github scope
// appends three lines computed from the github slot alone (design D4):
// effective is the saved slot only (the per-invocation --proxy flag is
// invisible to this report, same rule as civitai today), and the source
// reuses proxySource against the slot value. An empty effective value means
// HTTP(S)_PROXY (env) or a direct connection applies, which the source
// field makes observable. With --verbose a debug line names the civitai
// effective proxy and its source on stderr (prd R6).
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
			Github: proxyScopeStatus{
				Persisted: cfg.GithubProxy,
				Effective: cfg.GithubProxy,
				Source:    proxySource(cfg.GithubProxy),
			},
		})
	}
	fmt.Fprintf(g.stdout(), "Persisted proxy: %s\n", proxyDisplay(cfg.Proxy))
	fmt.Fprintf(g.stdout(), "Effective proxy: %s\n", proxyDisplay(effective))
	fmt.Fprintf(g.stdout(), "Source: %s\n", source)
	fmt.Fprintf(g.stdout(), "GitHub persisted: %s\n", proxyDisplay(cfg.GithubProxy))
	fmt.Fprintf(g.stdout(), "GitHub effective: %s\n", proxyDisplay(cfg.GithubProxy))
	fmt.Fprintf(g.stdout(), "GitHub source: %s\n", proxySource(cfg.GithubProxy))
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
