//go:build !windows

package report

// consoleWidth on non-Windows platforms returns 0 (no console query in
// M1; Windows is the primary target). ConsoleWidth applies the documented
// 80-column fallback. A TIOCGWINSZ ioctl can be added here later without
// touching callers.
func consoleWidth() int {
	return 0
}
