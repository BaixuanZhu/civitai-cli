package hashcache

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTemp creates a file with the given content in a test-owned temp
// directory and returns its path.
func writeTemp(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func TestComputeSHA256_KnownContent(t *testing.T) {
	// Small-input edge case: the streaming pipeline (open, copy, hex,
	// uppercase) must agree with the one-shot stdlib oracle. The expected
	// digest is derived from sha256.Sum256 rather than a hardcoded hex
	// constant so the test stays portable across platforms/toolchains.
	content := []byte("abc")
	path := writeTemp(t, "abc.bin", content)
	got, err := ComputeSHA256(path)
	if err != nil {
		t.Fatalf("ComputeSHA256: %v", err)
	}
	sum := sha256.Sum256(content)
	want := strings.ToUpper(hex.EncodeToString(sum[:]))
	if got != want {
		t.Errorf("digest = %s, want uppercase %s", got, want)
	}
}

func TestComputeSHA256_MatchesSum256(t *testing.T) {
	// 200003 bytes exercises several 32 KB buffer rounds and a partial
	// final block; the content is deterministic (no rand dependency).
	content := make([]byte, 200003)
	for i := range content {
		content[i] = byte(i*31 + 7)
	}
	path := writeTemp(t, "stream.bin", content)
	got, err := ComputeSHA256(path)
	if err != nil {
		t.Fatalf("ComputeSHA256: %v", err)
	}
	sum := sha256.Sum256(content)
	want := strings.ToUpper(hex.EncodeToString(sum[:]))
	if got != want {
		t.Errorf("streaming digest = %s, want direct sha256.Sum256 = %s", got, want)
	}
}

func TestComputeSHA256_EmptyFile(t *testing.T) {
	path := writeTemp(t, "empty.safetensors", nil)
	got, err := ComputeSHA256(path)
	if err != nil {
		t.Fatalf("ComputeSHA256: %v", err)
	}
	want := "E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855"
	if got != want {
		t.Errorf("empty-file digest = %s, want %s", got, want)
	}
}

func TestComputeSHA256_MissingPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.safetensors")
	if _, err := ComputeSHA256(path); err == nil {
		t.Fatalf("missing path must error")
	}
}
