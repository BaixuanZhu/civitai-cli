package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"civitai-cli/internal/api"
	"civitai-cli/internal/config"
)

// Login runs the login command: civ login. It reads an API key from stdin
// (interactive paste or a pipe), validates it with the favorites probe
// (GET /models?favorites=true&limit=1), and on success saves it to the
// config file. Exit codes: 0 saved, 1 empty/usage input, 2 network failure,
// 4 the key was rejected (401). The key is never echoed to stdout.
func Login(args []string) {
	executeWithExitCode("login", args, nil, func(ctx context.Context, g *globals, pos []string) error {
		return runLogin(ctx, g, pos, os.Stdin)
	}, loginExitCode, loginErrorHint)
}

// runLogin is the pure login implementation with an injectable stdin so
// tests can drive it with a strings.Reader (no terminal, no network).
func runLogin(ctx context.Context, g *globals, pos []string, stdin io.Reader) error {
	if len(pos) != 0 {
		return errors.New("login takes no arguments; it reads the API key from stdin")
	}
	fmt.Fprint(os.Stderr, "Paste API key: ")
	sc := bufio.NewScanner(stdin)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return fmt.Errorf("login: read stdin: %w", err)
		}
		return errors.New("no API key provided (empty input)")
	}
	key := strings.TrimSpace(sc.Text())
	if key == "" {
		return errors.New("no API key provided (empty input)")
	}

	// Probe with the candidate key. A copy of globals carries it as the
	// flag token so newAPIClient (the test-redirectable factory) builds a
	// client authenticated with the pasted key, not the saved one.
	probe := *g
	probe.Token = key
	c := newAPIClient(&probe)
	q := url.Values{}
	q.Set("favorites", "true")
	q.Set("limit", "1")
	if _, err := c.Get(ctx, "/models", q); err != nil {
		return err // 401 → loginExitCode → exit 4 (nothing saved); network → 2
	}

	if err := config.Save(config.Config{Token: key}); err != nil {
		return err
	}
	fmt.Fprintf(g.stdout(), "Logged in. API key saved to %s\n", config.Path())
	return nil
}

// loginExitCode maps a rejected candidate key to exit 4 regardless of the
// resolved globals token: login always probes WITH the pasted key, so a
// 401 means that key is invalid (the global hasToken heuristic does not
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

// Logout runs the logout command: civ logout. It deletes the config file
// (idempotent) and prints a friendly confirmation. Exit 0 even when no
// key was saved; exit 1 on a usage error or an unexpected removal failure.
func Logout(args []string) {
	execute("logout", args, nil, runLogout)
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
