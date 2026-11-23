package console

import (
	"strings"
	"testing"
)

func TestNewMenuState(t *testing.T) {
	// Multi: preselect toggles rows and parks the cursor on the first.
	s := newMenuState(3, []int{0, 2}, true)
	if s.cursor != 0 || !s.toggled[0] || s.toggled[1] || !s.toggled[2] {
		t.Errorf("multi preselect = cursor %d, toggled %v; want 0, [t f t]", s.cursor, s.toggled)
	}
	// Single: the marked row is only the initial cursor, never a toggle.
	s = newMenuState(2, []int{1}, false)
	if s.cursor != 1 || s.toggled[0] || s.toggled[1] {
		t.Errorf("single default = cursor %d, toggled %v; want 1, [f f]", s.cursor, s.toggled)
	}
	// Out-of-range marks are clamped (cursor) / dropped (toggles).
	s = newMenuState(2, []int{7}, true)
	if s.cursor != 1 || s.toggled[0] || s.toggled[1] {
		t.Errorf("stale default must clamp, got cursor %d, toggled %v", s.cursor, s.toggled)
	}
}

func TestStep(t *testing.T) {
	st := func(cursor int, toggled ...bool) menuState {
		return menuState{cursor: cursor, toggled: toggled}
	}
	cases := []struct {
		name  string
		start menuState
		k     key
		multi bool
		want  menuState
	}{
		{"up moves", st(1, false, false, false), key{kind: keyUp}, true, st(0, false, false, false)},
		{"up clamps at top", st(0, false, false), key{kind: keyUp}, true, st(0, false, false)},
		{"down moves", st(1, false, false, false), key{kind: keyDown}, true, st(2, false, false, false)},
		{"down clamps at bottom", st(2, false, false, false), key{kind: keyDown}, true, st(2, false, false, false)},
		{"space toggles on (multi)", st(1, false, false, false), key{kind: keySpace}, true, st(1, false, true, false)},
		{"space toggles off (multi)", st(1, false, true), key{kind: keySpace}, true, st(1, false, false)},
		{"space ignored (single)", st(1, false, false), key{kind: keySpace}, false, st(1, false, false)},
		{"a toggles all on", st(0, false, false, false), key{kind: keyRune, r: 'a'}, true, st(0, true, true, true)},
		{"a toggles all off", st(0, true, true), key{kind: keyRune, r: 'a'}, true, st(0, false, false)},
		{"a toggles mixed to all on", st(1, true, false), key{kind: keyRune, r: 'a'}, true, st(1, true, true)},
		{"a ignored (single)", st(1, false, false), key{kind: keyRune, r: 'a'}, false, st(1, false, false)},
		{"enter accepts single", st(1, false, false), key{kind: keyEnter}, false, menuState{cursor: 1, toggled: []bool{false, false}, done: true}},
		{"enter accepts toggled multi", st(0, true, false), key{kind: keyEnter}, true, menuState{cursor: 0, toggled: []bool{true, false}, done: true}},
		{"enter no-ops empty multi", st(0, false, false), key{kind: keyEnter}, true, st(0, false, false)},
		{"esc cancels", st(0, false), key{kind: keyEsc}, true, menuState{cursor: 0, toggled: []bool{false, false}, done: true, canceled: true}},
		{"q cancels", st(0, false), key{kind: keyRune, r: 'q'}, false, menuState{cursor: 0, toggled: []bool{false, false}, done: true, canceled: true}},
		{"ctrl-c cancels", st(0, false), key{kind: keyCancel}, true, menuState{cursor: 0, toggled: []bool{false, false}, done: true, canceled: true}},
		{"none ignored", st(1, false, false), key{kind: keyNone}, true, st(1, false, false)},
		{"left/right ignored", st(1, false, false), key{kind: keyLeft}, true, st(1, false, false)},
	}
	for _, c := range cases {
		got := step(c.start, c.k, c.multi)
		if got.cursor != c.want.cursor || got.done != c.want.done || got.canceled != c.want.canceled {
			t.Errorf("%s: state = cursor %d done %v canceled %v; want %d %v %v",
				c.name, got.cursor, got.done, got.canceled, c.want.cursor, c.want.done, c.want.canceled)
		}
		for i := range got.toggled {
			if got.toggled[i] != c.want.toggled[i] {
				t.Errorf("%s: toggled = %v; want %v", c.name, got.toggled, c.want.toggled)
				break
			}
		}
	}
}

func TestStepDoesNotAliasToggled(t *testing.T) {
	// Purity contract (design D4): step must not mutate the receiver's
	// backing array, or repeated steps on one state would double-apply.
	s := menuState{cursor: 0, toggled: []bool{false, true}}
	_ = step(s, key{kind: keySpace}, true)
	if s.toggled[0] {
		t.Errorf("step must not mutate the input state's toggles")
	}
}

func TestSelectionOf(t *testing.T) {
	sel := selectionOf(menuState{cursor: 1, toggled: []bool{true, false, true}}, true)
	if len(sel.Indexes) != 2 || sel.Indexes[0] != 0 || sel.Indexes[1] != 2 || sel.Canceled {
		t.Errorf("multi selection = %+v; want indexes [0 2]", sel)
	}
	sel = selectionOf(menuState{cursor: 1, toggled: []bool{false, false}}, false)
	if len(sel.Indexes) != 1 || sel.Indexes[0] != 1 || sel.Canceled {
		t.Errorf("single selection = %+v; want indexes [1]", sel)
	}
	sel = selectionOf(menuState{cursor: 1, toggled: []bool{true}, canceled: true}, true)
	if !sel.Canceled || sel.Indexes != nil {
		t.Errorf("canceled selection = %+v; want Canceled with no indexes", sel)
	}
}

func TestRenderFrameMulti(t *testing.T) {
	s := menuState{cursor: 0, toggled: []bool{true, false, false}}
	got := renderFrame(s, "Select platforms for civ skill install:", []string{
		"claude  /home/u/.claude/skills  [detected]",
		"zcode   /home/u/.zcode/skills",
		"codex   /home/u/.agents/skills",
	}, true)
	want := "Select platforms for civ skill install:\n" +
		"  > [✓] claude  /home/u/.claude/skills  [detected]\n" +
		"    [ ] zcode   /home/u/.zcode/skills\n" +
		"    [ ] codex   /home/u/.agents/skills\n" +
		multiFooter + "\n"
	if got != want {
		t.Errorf("multi frame:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderFrameSingle(t *testing.T) {
	s := menuState{cursor: 1, toggled: []bool{false, false}}
	got := renderFrame(s, "Scope (default user):", []string{
		"user (home directory)",
		"project (current directory)",
	}, false)
	want := "Scope (default user):\n" +
		"    user (home directory)\n" +
		"  > project (current directory)\n" +
		singleFooter + "\n"
	if got != want {
		t.Errorf("single frame:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderFrameConstantLineCount(t *testing.T) {
	// The in-place repaint moves the cursor up a fixed line count, so every
	// state of a frame must render the same number of lines (design D4).
	rows := []string{"a", "b", "c"}
	states := []menuState{
		{cursor: 0, toggled: []bool{false, false, false}},
		{cursor: 1, toggled: []bool{true, false, true}},
		{cursor: 2, toggled: []bool{true, true, true}},
	}
	for _, s := range states {
		for _, multi := range []bool{true, false} {
			if n := strings.Count(renderFrame(s, "T", rows, multi), "\n"); n != len(rows)+2 {
				t.Errorf("frame with cursor %d multi %v has %d lines; want %d", s.cursor, multi, n, len(rows)+2)
			}
		}
	}
}
