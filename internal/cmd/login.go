package cmd

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"civitai-cli/internal/api"
	"civitai-cli/internal/config"
)

// loginHelp is the curated `civ login help` / `-h` page (without the
// shared global block, which writeCommandHelp appends): the key candidate
// precedence, the history-safe stdin forms, and the login exit codes.
const loginHelp = `save an API token to the config file

Usage:
  civ login [--key <token> | <token>] [flags]

Options:
  --key <token>    API key to save; "-" reads the key from stdin
  -t <token>       Shorthand for --key

Examples:
  civ login
  civ login cifcYourApiKey123
  echo "$KEY" | civ login --key -

Notes:
  The key candidate is picked in this order: --key/-t flag, then one
  positional argument, then the global --token flag, then an interactive
  stdin prompt (never echoed). A key typed on the command line stays in
  shell history and process listings; prefer the prompt or the piped form.
  The candidate is verified against the API before anything is saved.
  Exit codes: 0 saved; 1 empty or invalid input; 2 network failure;
  4 key rejected.
`

// Login runs the login command: civ login. The API key candidate comes
// from the --key/-t flag, a single positional argument, or the global
// --token flag, in that precedence order; with no candidate it is read
// from stdin (interactive paste or a pipe, never echoed). A --key/-t
// value of "-" also reads the key from stdin. The candidate is validated
// with the favorites probe (GET /models?favorites=true&limit=1) and on
// success saved to the config file. Exit codes: 0 saved, 1 empty/usage
// input, 2 network failure, 4 the key was rejected (401). The key is
// never echoed to stdout.
func Login(args []string) {
	var o loginOptions
	executeWithExitCode("login", loginHelp, args, o.register, func(ctx context.Context, g *globals, pos []string) error {
		return runLogin(ctx, g, pos, &o, os.Stdin)
	}, loginExitCode, loginErrorHint)
}

// loginOptions carries the login-specific flags.
type loginOptions struct {
	key loginKeyFlag
}

// register binds the login flags to fs. --key and its -t shorthand share
// one value; a value of "-" means "read the key from stdin".
func (o *loginOptions) register(fs *flag.FlagSet) {
	fs.Var(&o.key, "key", "API key to save ('-' reads it from stdin)")
	fs.Var(&o.key, "t", "API key to save (shorthand for --key)")
}

// loginKeyFlag is a string flag value that records whether it was set, so
// an explicitly empty `--key ""` counts as an empty candidate (usage
// error) instead of "flag absent" falling through to the stdin prompt.
type loginKeyFlag struct {
	set bool
	val string
}

func (f *loginKeyFlag) String() string { return f.val }

func (f *loginKeyFlag) Set(s string) error {
	f.set = true
	f.val = s
	return nil
}

// runLogin is the pure login implementation with an injectable stdin so
// tests can drive it with a strings.Reader (no terminal, no network).
// Candidate precedence: --key/-t flag > single positional argument >
// global --token flag > stdin (interactive prompt or pipe). A --key/-t
// value of "-" redirects to stdin for a history-safe one-liner (the
// gh/curl "-" convention).
func runLogin(ctx context.Context, g *globals, pos []string, o *loginOptions, stdin io.Reader) error {
	if len(pos) > 1 {
		return errors.New("login takes at most one argument (the API key)")
	}
	candidate := ""
	readStdin := false
	switch {
	case o.key.set:
		if o.key.val == "-" {
			readStdin = true
		} else {
			candidate = o.key.val
		}
	case len(pos) == 1:
		candidate = pos[0]
	case g.Token != "":
		candidate = g.Token
	default:
		readStdin = true
	}

	var key string
	if readStdin {
		fmt.Fprint(os.Stderr, "Paste API key: ")
		sc := bufio.NewScanner(stdin)
		if !sc.Scan() {
			if err := sc.Err(); err != nil {
				return fmt.Errorf("login: read stdin: %w", err)
			}
			return errors.New("no API key provided (empty input)")
		}
		key = strings.TrimSpace(sc.Text())
	} else {
		key = strings.TrimSpace(candidate)
	}
	if key == "" {
		return errors.New("no API key provided (empty input)")
	}

	// Probe with the candidate key. A copy of globals carries it as the
	// flag token so newAPIClient (the test-redirectable factory) builds a
	// client authenticated with the candidate key, not the saved one.
	probe := *g
	probe.Token = key
	c := newAPIClient(&probe)
	q := url.Values{}
	q.Set("favorites", "true")
	q.Set("limit", "1")
	if _, err := c.Get(ctx, "/models", q); err != nil {
		return err // 401 → loginExitCode → exit 4 (nothing saved); network → 2
	}

	// Load-modify-save: rewrite only the token of the loaded config so
	// sibling fields (e.g. a future proxy setting) survive a login instead
	// of being wiped by a fresh Config.
	cfg, _ := config.Load()
	cfg.Token = key
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Fprintf(g.stdout(), "Logged in. API key saved to %s\n", config.Path())
	return nil
}

// loginExitCode maps a rejected candidate key to exit 4 regardless of the
// resolved globals token: login always probes WITH the candidate key, so
// a 401 means that key is invalid (the global hasToken heuristic does not
// apply). Every other class keeps the shared mapping.
func loginExitCode(err error, hasToken bool) int {
	var ue *api.UnauthorizedError
	if errors.As(err, &ue) {
		return 4
	}
	return exitCodeFor(err, hasToken)
}

// loginErrorHint gives key-specific guidance on a 401 and delegates other
// classes to the shared hints.
func loginErrorHint(err error, hasToken bool) {
	var ue *api.UnauthorizedError
	if errors.As(err, &ue) {
		fmt.Fprintln(os.Stderr, "hint: the API rejected this key; verify it under civitai.com user settings and run civ login again")
		return
	}
	printErrorHint(err, hasToken)
}

// logoutHelp is the curated `civ logout help` / `-h` page (without the
// shared global block, which writeCommandHelp appends). It states plainly
// that the whole config file goes, saved proxy included (runLogout calls
// config.Delete, and Config holds token and proxy).
const logoutHelp = `remove the saved API token

Usage:
  civ logout

Examples:
  civ logout
  civ login

Notes:
  Logout deletes the whole config file: a proxy saved via civ proxy set is
  cleared as well. To clear only the proxy and keep the token, run
  civ proxy unset. Idempotent: exit 0 even when nothing was saved.
`

// Logout runs the logout command: civ logout. It deletes the config file
// (idempotent) and prints a friendly confirmation. Exit 0 even when no
// key was saved; exit 1 on a usage error or an unexpected removal failure.
func Logout(args []string) {
	execute("logout", logoutHelp, args, nil, runLogout)
}

// runLogout is the pure logout implementation (testable without os.Exit).
func runLogout(ctx context.Context, g *globals, pos []string) error {
	if len(pos) != 0 {
		return errors.New("logout takes no arguments")
	}
	existed := false
	if p := config.Path(); p != "" {
		if _, err := os.Stat(p); err == nil {
			existed = true
		}
	}
	if err := config.Delete(); err != nil {
		return err
	}
	if existed {
		fmt.Fprintln(g.stdout(), "Logged out. API key removed.")
	} else {
		fmt.Fprintln(g.stdout(), "No saved API key; nothing to remove.")
	}
	return nil
}
