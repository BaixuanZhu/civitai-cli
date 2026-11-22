// Package app holds cross-cutting CLI infrastructure shared by all layers:
// the version stamp, the fatal-error helper, User-Agent construction, and
// the leveled stderr logger. It is a leaf package (imports nothing from
// internal/) so both api and cmd can depend on it without import cycles.
package app

import (
	"fmt"
	"os"
)

// Version is the CLI version stamp. Release builds override it via
// -ldflags "-X civitai-cli/internal/app.Version=<git describe>"; the default
// marks a development build.
var Version = "0.1.0-dev"

// repoURL is the canonical project location advertised in the User-Agent.
const repoURL = "https://github.com/BaixuanZhu/civitai-cli"

// Fail prints err to stderr and terminates the process with exit code 1
// (usage-class failure). It never returns.
func Fail(err error) {
	fmt.Fprintln(os.Stderr, "civ:", err)
	os.Exit(1)
}

// UserAgent returns the HTTP User-Agent every outbound request must carry:
// "civ/<Version> (+<repo-url>)".
func UserAgent() string {
	return "civ/" + Version + " (+" + repoURL + ")"
}
