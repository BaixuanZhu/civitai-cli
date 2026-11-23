// Command civ is the Civitai CLI: search models, inspect versions, and vet
// local files against the Civitai registry.
//
// main only routes: each subcommand lives in internal/cmd and owns its flag
// set, API calls, rendering, and exit-code mapping. Global flags (--json,
// --token, --proxy, --timeout, --verbose) are accepted after the command
// name only.
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"civitai-cli/internal/app"
	"civitai-cli/internal/cmd"
	"civitai-cli/internal/env"
	"civitai-cli/internal/upgrade"
)

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// Silent startup maintenance: clean a .bak left by the last upgrade, and
	// (release builds only) register the exe directory into the user PATH.
	// Dev builds inject Bootstrap=off and CIV_NO_BOOTSTRAP disables it anywhere.
	upgrade.CleanupStaleBak()
	// "unbootstrap" is the uninstaller's reverse-bootstrap (it removes the exe
	// dir from PATH), so skip the startup add for it rather than re-registering
	// the directory right before removing it.
	unbootstrap := len(os.Args) >= 2 && os.Args[1] == "unbootstrap"
	if !unbootstrap && app.BootstrapEnabled() {
		if err := env.EnsureUserPath(); err != nil {
			log.Printf("[WARN] PATH bootstrap failed: %v", err)
		}
	}

	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(1)
	}

	// Dispatch table: keep main free of any command logic; each case delegates
	// to internal/cmd (or upgrade for self-update).
	switch os.Args[1] {
	case "search":
		cmd.Search(os.Args[2:])
	case "show":
		cmd.Show(os.Args[2:])
	case "vet":
		cmd.Vet(os.Args[2:])
	case "sync":
		cmd.Sync(os.Args[2:])
	case "tags":
		cmd.Tags(os.Args[2:])
	case "login":
		cmd.Login(os.Args[2:])
	case "logout":
		cmd.Logout(os.Args[2:])
	case "proxy":
		cmd.Proxy(os.Args[2:])
	case "skill":
		cmd.Skill(os.Args[2:])
	case "upgrade":
		// upgrade parses its own --proxy flag inside Run, so it bypasses the
		// runner's help handling; main routes its help tokens directly (task
		// 11-22 design D4).
		if isUpgradeHelpArg(os.Args[2:]) {
			fmt.Print(upgrade.Help())
			return
		}
		upgrade.Run(os.Args[2:])
	case "completion":
		cmd.Completion(os.Args[2:])
	case "unbootstrap":
		// Internal maintenance hook invoked by the uninstaller (not listed in
		// usage): remove this exe's directory from the user PATH.
		if err := env.RemoveUserPath(); err != nil {
			log.Printf("[WARN] PATH removal failed: %v", err)
			os.Exit(1)
		}
	case "version", "-v", "--version":
		fmt.Println("civ", app.Version)
	case "help", "-h", "--help":
		// Only the literal "help" keyword takes a target: `civ help <name>`
		// routes to the curated page (cmd.WriteHelpPage), to upgrade's own
		// page for the upgrade name (cmd cannot import internal/upgrade,
		// design D4), or to the unknown-command error. The -h/--help flag
		// forms always print the overview, matching `civ <cmd> -h`.
		if os.Args[1] == "help" && len(os.Args) >= 3 {
			name := os.Args[2]
			if len(os.Args) == 3 {
				if cmd.WriteHelpPage(os.Stdout, name) {
					return
				}
				if name == "upgrade" {
					fmt.Print(upgrade.Help())
					return
				}
			} else {
				name = strings.Join(os.Args[2:], " ")
			}
			fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", name)
			usage(os.Stderr)
			os.Exit(1)
		}
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage(os.Stderr)
		os.Exit(1)
	}
}

// isUpgradeHelpArg reports whether args is exactly one help token: the
// bare `civ upgrade help` keyword or the -h/-help/--help flag form.
// upgrade bypasses the shared runner in internal/cmd (it has no flags to
// parse), so this check lives in main next to the routing; printing the
// page and returning normally keeps the stdout/exit-0 help contract.
func isUpgradeHelpArg(args []string) bool {
	if len(args) != 1 {
		return false
	}
	switch args[0] {
	case "help", "-h", "-help", "--help":
		return true
	}
	return false
}

// usage prints the command overview and global flags to w.
func usage(w io.Writer) {
	fmt.Fprint(w, `civ - Civitai CLI (search, inspect, and audit models from the terminal)

Usage:
  civ <command> [flags]

Commands:
  search <query>   Search models (--type --base --sort --period --tag --user --limit)
  show <modelId>   Show model header and version table (-v <versionId> drills in; --prompts for recipes)
  vet <path|hash>  Audit a local file (or SHA256/AutoV2 hash) against the Civitai registry (--prompts)
  sync <dir>       Batch-audit a directory (--ext --format table|csv|json --concurrency --rehash)
  tags <query>     Tag name completion (--limit)
  login [key]      Save an API token: --key/-t <key>, one positional <key>, --token, or a stdin prompt
  logout           Remove the saved API token
  proxy <action>   Save/show/clear the outbound proxy (set/get/unset; bare = status)
  skill <action>   Install/remove/update the civ-prompt skill (claude/zcode/codex; interactive or --platform)
  upgrade          Self-update from the latest GitHub Release ([--proxy <url>])
  completion <shell>  Print a shell completion script (bash, zsh, fish, powershell)
  version          Print the civ version
  help             Print this help

Run "civ <command> help" or "civ help <command>" (or civ <command> -h) for flags and examples.

Global flags (after the command name):
  --json           Machine-readable output (human decoration off)
  --token <key>    API token; overrides CIVITAI_API_KEY and the saved config (anonymous by default)
  --proxy <url>    Outbound proxy for this invocation; overrides the saved proxy and HTTP(S)_PROXY
  --timeout <dur>  Per-request HTTP timeout (default 30s)
  --verbose        Verbose [INFO]/[DEBUG] logging to stderr

Login:
  Key precedence: --key/-t flag > one positional argument > --token > stdin
  prompt (a pipe works too); --key - / -t - reads the key from stdin. A key
  typed on the command line is exposed via shell history and the process
  list; prefer piping (echo "$KEY" | civ login) or civ login --key - (both
  read the key from stdin, never echoed) when that matters.

Proxy:
  Two independent slots: civ proxy set <url> persists the civitai slot
  (http/https/socks5/socks5h), which routes the data commands (search, show,
  vet, sync, tags); civ proxy set --github <url> persists the GitHub slot,
  which routes only civ upgrade. Data commands resolve --proxy flag >
  civitai slot > HTTP_PROXY/HTTPS_PROXY env. civ upgrade connects directly
  by default and resolves --proxy flag > GitHub slot > env; get/unset show or
  clear a slot, and bare civ proxy prints the status of both.

Environment:
  CIVITAI_API_KEY  API token (below --token, above the saved config)
  CIV_NO_CACHE     Non-empty bypasses the sync hash cache (read and write)
  CIV_NO_BOOTSTRAP Non-empty disables the startup PATH registration

Exit codes:
  0 success   1 usage error   2 network failure   3 hash not registered (vet)   4 token rejected
`)
}
