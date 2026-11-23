// Package console renders interactive arrow-key selection menus on the
// terminal: a multi-select (up/down move, space toggles, 'a' toggles all,
// enter accepts) and a single-select list, driven by VT input sequences and
// repainted in place with ANSI cursor moves.
//
// Raw console access is hand-written syscall only — kernel32
// GetStdHandle/GetConsoleMode/SetConsoleMode on Windows — mirroring
// internal/env, so the package keeps the project's zero-third-party-
// dependency contract. It is a leaf that imports no other internal package
// and knows nothing about the commands that use it: callers own the wording
// and fall back to text prompts when IsTerminal is false or a menu returns
// an error. Menus draw on stderr only; command results stay on stdout.
package console

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrNotSupported reports that arrow-key menus are unavailable: the build
// has no raw-console support (non-Windows) or stdin/stderr is not a console
// with VT support. Callers use it to fall back to text prompts instead of
// failing the command.
var ErrNotSupported = errors.New("console: raw console mode not supported")

// Selection is the outcome of a menu run: the accepted row indexes (in row
// order) and whether the user canceled.
type Selection struct {
	// Indexes lists the accepted rows: the toggled rows for SelectMulti, or
	// the single highlighted row for SelectOne. Empty only when Canceled.
	Indexes []int
	// Canceled reports an explicit cancel (Esc, q, or Ctrl+C).
	Canceled bool
}

// SelectMulti runs an arrow-key multi-select menu on the terminal: up/down
// move, space toggles the highlighted row, 'a' toggles every row, Enter
// accepts (the menu stays open while nothing is toggled), and Esc/q/Ctrl+C
// cancel. preselect lists the row indexes initially toggled, with the
// cursor on the first. The frame is drawn on stderr. Setup or read failures
// return an error (wrapping ErrNotSupported for setup) so the caller can
// fall back to text prompts; a cancel is a normal Selection, not an error.
func SelectMulti(title string, rows []string, preselect []int) (Selection, error) {
	return runMenu(title, rows, preselect, true)
}

// SelectOne runs an arrow-key single-select menu: up/down move, Enter
// accepts the highlighted row; space and 'a' have no effect. defaultIdx is
// the initially highlighted row, clamped into range. Cancel semantics and
// the error contract match SelectMulti.
func SelectOne(title string, rows []string, defaultIdx int) (Selection, error) {
	return runMenu(title, rows, []int{defaultIdx}, false)
}

// runMenu is the shared SelectMulti/SelectOne loop (design D3/D4): enter
// raw mode, draw the frame, then read -> decode -> step -> repaint until
// the reducer finishes, restoring the console on every exit path. It never
// panics on odd input — unknown bytes decode to ignored keys.
func runMenu(title string, rows []string, marked []int, multi bool) (Selection, error) {
	if len(rows) == 0 {
		return Selection{}, fmt.Errorf("console: menu %q has no rows", title)
	}
	restore, err := enterRawMode()
	if err != nil {
		return Selection{}, err
	}
	defer restore()

	state := newMenuState(len(rows), marked, multi)
	fmt.Fprint(os.Stderr, renderFrame(state, title, rows, multi))
	kr := &keyReader{r: os.Stdin}
	for {
		k, err := kr.next()
		if err != nil {
			// Erase the half-driven frame so the caller's fallback prompts
			// do not print below a stale menu skeleton.
			clearFrame(os.Stderr, len(rows))
			return Selection{}, fmt.Errorf("console: read menu input: %w", err)
		}
		state = step(state, k, multi)
		if state.done {
			break
		}
		repaintFrame(os.Stderr, state, title, rows, multi)
	}
	if state.canceled {
		clearFrame(os.Stderr, len(rows))
		return Selection{Canceled: true}, nil
	}
	// Final repaint leaves the accepted choices visible as the record of
	// what was selected; the blank line separates them from later output.
	repaintFrame(os.Stderr, state, title, rows, multi)
	fmt.Fprintln(os.Stderr)
	return selectionOf(state, multi), nil
}

// repaintFrame rewrites the whole frame in place: erase the previous frame,
// then print the new one (full row-block repaint; menus are a few rows).
func repaintFrame(out io.Writer, s menuState, title string, rows []string, multi bool) {
	clearFrame(out, len(rows))
	fmt.Fprint(out, renderFrame(s, title, rows, multi))
}

// clearFrame erases the last drawn frame. A frame occupies one title line
// plus len(rows) row lines plus one footer line, each newline-terminated,
// so the cursor sits on the line after it: moving up rows+2 lines and
// clearing to the end of screen wipes exactly the frame.
func clearFrame(out io.Writer, rows int) {
	fmt.Fprintf(out, "\x1b[%dA\x1b[J", rows+2)
}

// keyReader turns a raw byte stream into decoded keys, buffering partial
// escape sequences across reads (a split "\x1b[" can arrive as two reads).
type keyReader struct {
	r   io.Reader
	buf []byte
}

// next returns the next meaningful key, reading more bytes as needed and
// silently consuming ignorable input (unknown letters and sequences).
func (kr *keyReader) next() (key, error) {
	for {
		k, n := decodeKey(kr.buf)
		switch {
		case k.kind == keyPending || n == 0: // incomplete or empty buffer
			if err := kr.fill(); err != nil {
				return key{}, err
			}
		case k.kind == keyNone: // consumed but ignorable
			kr.buf = kr.buf[n:]
		default:
			kr.buf = kr.buf[n:]
			return k, nil
		}
	}
}

// fill reads one chunk, appending after any residual partial sequence.
func (kr *keyReader) fill() error {
	var tmp [32]byte
	m, err := kr.r.Read(tmp[:])
	if m > 0 {
		kr.buf = append(kr.buf, tmp[:m]...)
	}
	return err
}
