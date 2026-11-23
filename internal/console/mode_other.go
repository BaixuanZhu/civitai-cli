//go:build !windows

package console

import (
	"fmt"
	"syscall"
)

// IsTerminal reports whether fd is an interactive character device. Windows
// is the primary target and gets the precise GetConsoleMode check; elsewhere
// this stat-based heuristic applies (the report/width_* precedent for
// splitting behavior by GOOS). It deliberately uses syscall.Fstat rather
// than os.NewFile(fd).Stat: os.NewFile registers a finalizer that closes
// the descriptor on GC, which would silently kill the caller's stdin.
func IsTerminal(fd uintptr) bool {
	var st syscall.Stat_t
	if err := syscall.Fstat(int(fd), &st); err != nil {
		return false
	}
	return st.Mode&syscall.S_IFMT == syscall.S_IFCHR
}

// enterRawMode is the non-Windows stub: raw mode is not implemented (M1
// targets Windows), so menus report ErrNotSupported and callers fall back
// to text prompts. A termios implementation can drop in here later without
// touching callers or tests.
func enterRawMode() (func(), error) {
	return nil, fmt.Errorf("%w: raw mode is not implemented on this platform", ErrNotSupported)
}
