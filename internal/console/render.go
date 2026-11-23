package console

import "strings"

// Footer hint lines documenting the key bindings (design D4). ASCII only:
// zh-CN classic conhost codepages would mojibake non-ASCII symbols (the
// project's all-English console lesson). The '[✓]' toggle marker below is
// the one user-approved non-ASCII exception (2026-11-23): U+2713 is
// neutral-width, so it cannot break column alignment the way ambiguous-
// width glyphs (●, ❯) do on CJK terminals.
const (
	multiFooter  = "  up/down move, space toggle, a all, enter accept, q/esc cancel"
	singleFooter = "  up/down move, enter accept, q/esc cancel"
)

// menuState is the state of an open menu: the highlighted row, the per-row
// toggle flags (multi-select only; the row count is len(toggled)), and the
// terminal conditions. step advances it as a pure function.
type menuState struct {
	cursor   int
	toggled  []bool
	done     bool
	canceled bool
}

// newMenuState builds the opening state for n rows. marked lists the
// initially toggled rows for multi-select, or the initially highlighted row
// for single-select; out-of-range indexes are clamped/dropped so a stale
// default can never crash the menu.
func newMenuState(n int, marked []int, multi bool) menuState {
	s := menuState{toggled: make([]bool, n)}
	if len(marked) > 0 {
		s.cursor = clampIndex(marked[0], n)
	}
	if multi {
		for _, i := range marked {
			if i >= 0 && i < n {
				s.toggled[i] = true
			}
		}
	}
	return s
}

// clampIndex bounds v to [0, n-1]; n <= 0 yields 0.
func clampIndex(v, n int) int {
	if n <= 0 {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v >= n {
		return n - 1
	}
	return v
}

// step applies one decoded key to the menu state (design D4 reducer):
// arrows move the cursor with clamping at both ends, space toggles the
// cursor row (multi-select only), 'a' flips between all-on and all-off,
// Enter accepts — a multi-select with nothing toggled stays open (the
// menu-idiom no-op recorded as a design deviation) — and Esc/'q'/Ctrl+C
// cancel. The receiver is left untouched (toggles are copied), keeping the
// function pure for table tests. Pure, offline-tested.
func step(s menuState, k key, multi bool) menuState {
	t := make([]bool, len(s.toggled))
	copy(t, s.toggled)
	s.toggled = t
	n := len(t)
	switch k.kind {
	case keyUp:
		if s.cursor > 0 {
			s.cursor--
		}
	case keyDown:
		if s.cursor < n-1 {
			s.cursor++
		}
	case keySpace:
		if multi {
			t[s.cursor] = !t[s.cursor]
		}
	case keyRune:
		switch k.r {
		case 'a':
			if multi {
				all := true
				for _, on := range t {
					if !on {
						all = false
						break
					}
				}
				for i := range t {
					t[i] = !all
				}
			}
		case 'q':
			s.canceled, s.done = true, true
		}
	case keyEnter:
		if !multi {
			s.done = true
			return s
		}
		for _, on := range t {
			if on {
				s.done = true
				break
			}
		}
	case keyEsc, keyCancel:
		s.canceled, s.done = true, true
	}
	return s
}

// selectionOf converts a finished state into the public Selection: the
// toggled rows for multi-select, or the highlighted row for single-select.
func selectionOf(s menuState, multi bool) Selection {
	if s.canceled {
		return Selection{Canceled: true}
	}
	if !multi {
		return Selection{Indexes: []int{s.cursor}}
	}
	var idx []int
	for i, on := range s.toggled {
		if on {
			idx = append(idx, i)
		}
	}
	return Selection{Indexes: idx}
}

// renderFrame draws one full menu frame as plain text lines: the title, one
// line per row (indent, '>' cursor marker or blank gutter, '[✓]'/'[ ]'
// toggle for multi-select, row label), and the footer hint. The line count
// is constant for a given row set — state only changes markers within a
// line — which is what lets the caller repaint in place with a fixed
// cursor-up count. Pure function, golden-tested offline (design D4).
func renderFrame(s menuState, title string, rows []string, multi bool) string {
	var b strings.Builder
	b.WriteString(title)
	b.WriteByte('\n')
	for i, row := range rows {
		b.WriteString("  ")
		if i == s.cursor {
			b.WriteString("> ")
		} else {
			b.WriteString("  ")
		}
		if multi {
			if s.toggled[i] {
				b.WriteString("[✓] ")
			} else {
				b.WriteString("[ ] ")
			}
		}
		b.WriteString(row)
		b.WriteByte('\n')
	}
	if multi {
		b.WriteString(multiFooter)
	} else {
		b.WriteString(singleFooter)
	}
	b.WriteByte('\n')
	return b.String()
}
