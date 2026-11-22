// Package testutil provides fixture-loading helpers shared by unit tests.
// It is imported only from _test.go files; no production code depends on
// it. Fixtures live in testdata/ at the repository root (see
// testdata/FIXTURES.md for provenance); tests never touch the network.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// LoadFixture reads testdata/<name> from the repository root, locating
// the root by walking up from the test's working directory until go.mod
// is found. It fails the test immediately when the fixture is missing.
func LoadFixture(t *testing.T, name string) []byte {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("testutil: getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			b, err := os.ReadFile(filepath.Join(dir, "testdata", name))
			if err != nil {
				t.Fatalf("testutil: load fixture %s: %v", name, err)
			}
			return b
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("testutil: repository root (go.mod) not found above working directory")
		}
		dir = parent
	}
}
