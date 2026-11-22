// Command civ is the Civitai CLI: search models, inspect versions, and vet
// local files against the Civitai registry.
//
// main only routes: each subcommand lives in internal/cmd and owns its flag
// set, API calls, rendering, and exit-code mapping. Global flags (--json,
// --token, --timeout, --verbose) are accepted after the command name only.
package main

import (
	"fmt"
	"io"
	"log"
	"os"

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
	if app.BootstrapEnabled() {
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
	case "upgrade":
		upgrade.Run()
	case "version", "-v", "--version":
		fmt.Println("civ", app.Version)
	case "help", "-h", "--help":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage(os.Stderr)
		os.Exit(1)
	}
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
  login            Save an API token (validated via a favorites probe)
  logout           Remove the saved API token
  upgrade          Self-update from the latest GitHub Release
  version          Print the civ version
  help             Print this help

Global flags (after the command name):
  --json           Machine-readable output (human decoration off)
  --token <key>    API token; overrides CIVITAI_API_KEY and the saved config (anonymous by default)
  --timeout <dur>  Per-request HTTP timeout (default 30s)
  --verbose        Verbose [INFO]/[DEBUG] logging to stderr

Environment:
  CIVITAI_API_KEY  API token (below --token, above the saved config)
  CIV_NO_CACHE     Non-empty bypasses the sync hash cache (read and write)
  CIV_NO_BOOTSTRAP Non-empty disables the startup PATH registration

Exit codes:
  0 success   1 usage error   2 network failure   3 hash not registered (vet)   4 token rejected
`)
}
