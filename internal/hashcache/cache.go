// Package hashcache computes local file hashes for the vet and sync
// commands. M2 ships only the streaming SHA256 computation; M3 adds the
// path/size/mtime cache (os.UserCacheDir()/civ/cache/hashes.json) on top
// without changing this function's contract.
package hashcache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// hashBufSize is the streaming copy buffer (32 KB, parent design §3):
// multi-GB checkpoints are hashed with bounded memory, never read whole.
const hashBufSize = 32 * 1024

// ComputeSHA256 streams the file at path through SHA256 and returns the
// uppercase hex digest (the API's canonical hash form). The file is
// copied into the digest with a 32 KB buffer, so memory use stays
// constant regardless of file size. Open/read failures are returned
// wrapped with the path for actionable messages.
func ComputeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("hashcache: open %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.CopyBuffer(h, f, make([]byte, hashBufSize)); err != nil {
		return "", fmt.Errorf("hashcache: read %s: %w", path, err)
	}
	return strings.ToUpper(hex.EncodeToString(h.Sum(nil))), nil
}
