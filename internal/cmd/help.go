package cmd

import (
	"fmt"
	"io"
	"strings"
)

// globalHelpBlock is the single source of the global flags section appended
// to every command help page. It is defined once (never copy-pasted into a
// command page) so adding a global flag cannot drift the pages apart.
const globalHelpBlock = `Global flags (accepted after the command name):
  --json           Machine-readable output (human decoration off)
  --token <key>    API token; overrides CIVITAI_API_KEY and the saved config
  --proxy <url>    Outbound proxy for this invocation; overrides the saved proxy and env
  --timeout <dur>  Per-request HTTP timeout (default 30s)
  --verbose        Verbose [INFO]/[DEBUG] logging to stderr
`

// helpPages is the single index from a command name to its curated page
// body. It backs WriteHelpPage (`civ help <name>`): a new command page must
// join this map to become reachable, so the index cannot drift from the
// pages (prd R4). upgrade is deliberately absent — cmd cannot import
// internal/upgrade (design D4); main.go special-cases that name.
var helpPages = map[string]string{
	"search":     searchHelp,
	"show":       showHelp,
	"recipe":     recipeHelp,
	"resolve":    resolveHelp,
	"compare":    compareHelp,
	"vet":        vetHelp,
	"sync":       syncHelp,
	"tags":       tagsHelp,
	"login":      loginHelp,
	"logout":     logoutHelp,
	"proxy":      proxyHelp,
	"skill":      skillHelp,
	"completion": completionHelp,
}

// WriteHelpPage renders the named command's curated help page to w — the
// "civ <name> -" title line, the command-owned body, then the shared global
// flags block — and reports whether name is a known command page. It is the
// `civ help <name>` entry point that main routes into; the `civ <name> help`
// entry point goes through the runner's isBareHelp interception instead, and
// both paths must render byte-identical output (prd AC2). On a miss it
// writes nothing and returns false so main can fall through to the
// unknown-command error path.
func WriteHelpPage(w io.Writer, name string) bool {
	body, ok := helpPages[name]
	if !ok {
		return false
	}
	writeCommandHelp(w, name, body)
	return true
}

// writeCommandHelp prints a command help page to w: the "civ <name> -"
// title line, the command-owned body, then the shared global flags block.
// body's first line is the one-line purpose; the rest carries the Usage /
// Options / Examples sections in that order (see task 11-22-command-help
// design D3). Help pages go to stdout with exit 0 — never stderr.
func writeCommandHelp(w io.Writer, name, body string) {
	purpose, rest, _ := strings.Cut(strings.TrimRight(body, "\n"), "\n")
	rest = strings.TrimSpace(rest)
	fmt.Fprintf(w, "civ %s - %s\n", name, purpose)
	if rest != "" {
		fmt.Fprintf(w, "\n%s\n", rest)
	}
	fmt.Fprintf(w, "\n%s", globalHelpBlock)
}

// isBareHelp reports whether pos is exactly one "help" positional — the
// `civ <command> help` entry point. Multi-positionals (search queries like
// "help me") and flag values never match, so the literal word stays usable.
func isBareHelp(pos []string) bool {
	return len(pos) == 1 && pos[0] == "help"
}
