// Package cmd implements the civ subcommands. Each command owns its flag
// set, calls internal/api for data, renders via internal/report, and maps
// failures to process exit codes through the shared execute runner.
// Command logic lives in pure run functions (error-returning, no
// os.Exit) so tests can drive them against httptest servers.
package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"civitai-cli/internal/api"
	"civitai-cli/internal/app"
	"civitai-cli/internal/config"
)

// globals carries the global flags shared by every subcommand. Global
// flags are accepted only AFTER the command name (civ search foo --json);
// main's dispatch rejects the pre-command position (design D1).
type globals struct {
	JSON    bool
	Token   string
	Proxy   string
	Timeout time.Duration
	Verbose bool

	out io.Writer // test injection point; nil means os.Stdout
}

// registerGlobalFlags binds the global flags to fs. --verbose has no
// short alias because show's -v carries version semantics (design D6).
func registerGlobalFlags(fs *flag.FlagSet, g *globals) {
	fs.BoolVar(&g.JSON, "json", false, "machine-readable output (human decoration off)")
	fs.StringVar(&g.Token, "token", "", "API token; overrides CIVITAI_API_KEY (anonymous by default)")
	fs.StringVar(&g.Proxy, "proxy", "", "proxy URL for this invocation; overrides the saved proxy and HTTP(S)_PROXY")
	fs.DurationVar(&g.Timeout, "timeout", 30*time.Second, "per-request HTTP timeout")
	fs.BoolVar(&g.Verbose, "verbose", false, "verbose [INFO]/[DEBUG] logging to stderr")
}

// resolveToken implements the full token priority (parent design D8):
// --token flag > CIVITAI_API_KEY environment variable > saved config file.
// The config layer is delegated to internal/config so login/logout and the
// command seam share one resolution chain.
func (g *globals) resolveToken() string {
	return config.ResolveToken(g.Token)
}

// stdout returns the command result writer (os.Stdout unless a test
// injected a buffer). Logs and warnings never go here.
func (g *globals) stdout() io.Writer {
	if g.out != nil {
		return g.out
	}
	return os.Stdout
}

// clientOptions returns the api options shared by every client factory:
// the resolved token, the per-request timeout, and the effective proxy
// (config.ResolveProxy applies --proxy flag > saved config; empty falls
// back to env/direct, invalid warns and degrades — task 11-22 design
// D2/D6). With --verbose a debug line names the effective proxy and its
// source (flag or config) so ad-hoc --proxy overrides are observable on
// data commands (prd R6). newAPIClient and newSyncClient both build on it
// so the two factory seams cannot drift apart.
func clientOptions(g *globals) []api.Option {
	proxyURL := config.ResolveProxy(g.Proxy)
	if proxyURL != "" {
		source := "config"
		if strings.TrimSpace(g.Proxy) != "" {
			source = "flag"
		}
		app.Debugf("proxy: routing through %s (source: %s)", proxyURL, source)
	}
	return []api.Option{
		api.WithToken(g.resolveToken()),
		api.WithTimeout(g.Timeout),
		api.WithProxy(proxyURL),
	}
}

// newAPIClient is the client factory used by all commands. Tests replace
// it to point the same command code at an httptest server.
var newAPIClient = func(g *globals) *api.Client {
	return api.NewClient(clientOptions(g)...)
}

// execute is the shared command runner: parse flags, apply verbose mode,
// run the command, map any error to an exit code. help is the command's
// curated help page (task 11-22-command-help): it is printed to stdout with
// exit 0 for both the -h/-help/--help flag and a bare "help" positional,
// before any run work (prompt, network, config). register binds
// command-specific flags before parsing; run receives the positional
// arguments left after flags. Commands use the global exit-code and hint
// mapping; vet overrides both via executeWithExitCode.
func execute(name string, help string, args []string, register func(*flag.FlagSet), run func(ctx context.Context, g *globals, pos []string) error) {
	executeWithExitCode(name, help, args, register, run, exitCodeFor, printErrorHint)
}

// executeWithExitCode is execute with injectable exit-code and hint
// mappers — the minimal command-level override mechanism. help is the
// curated help page printed for -h/--help and a bare "help" positional
// (stdout, exit 0); the Go default flag dump is suppressed for every path.
// exitMap returns the process exit code for err (hasToken reports whether a
// token was resolved); hint prints actionable guidance to stderr after the
// error line. vet passes vetExitCode/vetErrorHint so NotFoundError means
// exit 3 ("hash not registered") instead of the global usage-class exit 1;
// search/show/tags keep the global semantics through execute.
func executeWithExitCode(name string, help string, args []string, register func(*flag.FlagSet), run func(ctx context.Context, g *globals, pos []string) error, exitMap func(error, bool) int, hint func(error, bool)) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {} // no default dump; curated help or app.Fail owns output
	var g globals
	registerGlobalFlags(fs, &g)
	if register != nil {
		register(fs)
	}
	if err := fs.Parse(permuteArgs(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) { // -h / -help / --help
			writeCommandHelp(os.Stdout, name, help)
			os.Exit(0)
		}
		app.Fail(err) // usage-class failure, exit 1
	}
	if isBareHelp(fs.Args()) { // `civ <command> help`, before any run work
		writeCommandHelp(os.Stdout, name, help)
		os.Exit(0)
	}
	app.SetVerbose(g.Verbose)

	if err := run(context.Background(), &g, fs.Args()); err != nil {
		hasToken := g.resolveToken() != ""
		fmt.Fprintln(os.Stderr, "civ:", err)
		hint(err, hasToken)
		os.Exit(exitMap(err, hasToken))
	}
}

// exitCodeFor maps an error to the process exit code (design D3):
// 0 success, 1 usage-class (including anonymous 401 on personal-state
// parameters and M1 not-found), 2 network failure, 4 rejected token.
// Exit 3 is vet's not-found semantics; vetExitCode overrides this default.
func exitCodeFor(err error, hasToken bool) int {
	if err == nil {
		return 0
	}
	var ne *api.NetworkError
	if errors.As(err, &ne) {
		return 2
	}
	var ue *api.UnauthorizedError
	if errors.As(err, &ue) {
		if hasToken {
			return 4
		}
		return 1
	}
	return 1
}

// printErrorHint adds actionable guidance after the error line.
func printErrorHint(err error, hasToken bool) {
	var ue *api.UnauthorizedError
	if errors.As(err, &ue) {
		if hasToken {
			fmt.Fprintln(os.Stderr, "hint: the API rejected the token; check --token / CIVITAI_API_KEY (run civ login)")
		} else {
			fmt.Fprintln(os.Stderr, "hint: this parameter requires an API token (--token or CIVITAI_API_KEY)")
		}
	}
	var nfe *api.NotFoundError
	if errors.As(err, &nfe) {
		fmt.Fprintln(os.Stderr, "hint: check the id; the model or version may have been removed")
	}
}

// permuteArgs reorders args so flags precede positional arguments. The
// stdlib flag package stops parsing at the first non-flag argument, but
// the plan's UX puts the query first (civ search stocking --type lora,
// civ show 182296 -v 482391). Flag/value pairs are kept together using
// the registered FlagSet; "--" forces everything after it positional.
func permuteArgs(fs *flag.FlagSet, args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(a) > 1 && strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			name := strings.TrimLeft(a, "-")
			if strings.Contains(name, "=") {
				continue // --flag=value carries its own value
			}
			if f := fs.Lookup(name); f != nil {
				if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
					continue // boolean flags stand alone
				}
				if i+1 < len(args) {
					i++
					flags = append(flags, args[i]) // consume the flag's value
				}
			}
			continue
		}
		positional = append(positional, a)
	}
	return append(flags, positional...)
}
