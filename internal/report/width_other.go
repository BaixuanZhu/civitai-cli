//go:build !windows

package report

import "civitai-cli/internal/console"

// consoleWidth on non-Windows platforms reuses the existing
// console.IsTerminal heuristic (Windows is the primary target; no
// TIOCGWINSZ ioctl is added): an interactive character device reports the
// documented 80-column fallback width so the common normalizer marks it
// as a console, while a pipe or regular file returns 0 and takes the
// non-console fallback. A real width query can be added here later
// without touching callers.
func consoleWidth(fd uintptr) int {
	if console.IsTerminal(fd) {
		return fallbackConsoleWidth
	}
	return 0
}
