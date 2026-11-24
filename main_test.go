package main

import (
	"bytes"
	"strings"
	"testing"
)

// wantUsageGroups is the ordered task-oriented inventory the top-level help
// must present. Every public top-level command appears in exactly one group;
// unbootstrap is internal and stays hidden (task 11-24-grouped-help H1/H4).
var wantUsageGroups = []struct {
	name     string
	commands []string
}{
	{"Discover", []string{"search", "show", "compare", "tags"}},
	{"Recipes", []string{"recipe"}},
	{"Local models", []string{"vet", "sync", "resolve"}},
	{"Utilities", []string{"login", "logout", "proxy", "skill", "upgrade", "completion", "version", "help"}},
}

// usageCommandList isolates the Commands block from the rendered overview so
// the structural assertions cannot accidentally match reference-section text.
func usageCommandList(t *testing.T, out string) []string {
	t.Helper()
	start := strings.Index(out, "Commands:\n")
	end := strings.Index(out, "\n\nRun \"civ <command> help\"")
	if start < 0 || end < 0 || end <= start {
		t.Fatalf("usage output does not contain the expected Commands block:\n%s", out)
	}
	return strings.Split(out[start+len("Commands:\n"):end], "\n")
}

// TestUsageGroupedCommandList pins the task-oriented grouping: the four
// headings in order, each command under its reviewed group exactly once, and
// no hidden or excluded command advertised (AC1/AC2/AC4).
func TestUsageGroupedCommandList(t *testing.T) {
	var buf bytes.Buffer
	usage(&buf)
	lines := usageCommandList(t, buf.String())

	var gotGroups []string
	got := map[string][]string{}
	current := ""
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "    "):
			fields := strings.Fields(line)
			if len(fields) == 0 || current == "" {
				t.Fatalf("command row outside a group: %q", line)
			}
			got[current] = append(got[current], fields[0])
		case strings.HasPrefix(line, "  ") && strings.HasSuffix(line, ":"):
			current = strings.TrimSuffix(strings.TrimSpace(line), ":")
			gotGroups = append(gotGroups, current)
			if _, seen := got[current]; seen {
				t.Errorf("duplicate group heading %q", current)
			}
			got[current] = nil
		case strings.TrimSpace(line) == "":
			// Blank separators between groups are allowed.
		default:
			t.Fatalf("unexpected line inside the Commands block: %q", line)
		}
	}

	if len(gotGroups) != len(wantUsageGroups) {
		t.Fatalf("group order = %v, want %d groups", gotGroups, len(wantUsageGroups))
	}
	for i, want := range wantUsageGroups {
		if gotGroups[i] != want.name {
			t.Errorf("group %d = %q, want %q", i, gotGroups[i], want.name)
		}
		if strings.Join(got[want.name], " ") != strings.Join(want.commands, " ") {
			t.Errorf("%s commands = %v, want %v", want.name, got[want.name], want.commands)
		}
	}

	seen := map[string]string{}
	for group, commands := range got {
		for _, command := range commands {
			if prev, ok := seen[command]; ok {
				t.Errorf("command %q appears in both %q and %q", command, prev, group)
			}
			seen[command] = group
		}
	}
	for _, forbidden := range []string{"unbootstrap", "export", "library", "index", "doctor", "outdated", "pack", "workflow", "context"} {
		if _, ok := seen[forbidden]; ok {
			t.Errorf("grouped help must not advertise %q", forbidden)
		}
	}
}

// TestUsageSectionsPreserved pins the non-command-list overview structure the
// grouping task must leave intact (H2/AC3): title, usage, grouped commands,
// the help pointer, then the reference sections in their existing order.
func TestUsageSectionsPreserved(t *testing.T) {
	var buf bytes.Buffer
	usage(&buf)
	out := buf.String()

	markers := []string{
		"civ - Civitai CLI",
		"\nUsage:\n  civ <command> [flags]\n",
		"\nCommands:\n",
		"\nRun \"civ <command> help\"",
		"\nGlobal flags (after the command name):\n",
		"\nLogin:\n",
		"\nProxy:\n",
		"\nEnvironment:\n",
		"\nExit codes:\n",
		"\n  0 success   1 usage error   2 network failure   3 hash not registered (vet)   4 token rejected   5 not ready (resolve)\n",
	}
	last := -1
	for _, marker := range markers {
		idx := strings.Index(out, marker)
		if idx < 0 {
			t.Fatalf("usage output missing %q:\n%s", marker, out)
		}
		if idx <= last {
			t.Errorf("section %q appears out of order", marker)
		}
		last = idx
	}
}
