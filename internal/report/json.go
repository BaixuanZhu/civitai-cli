package report

import (
	"encoding/json"
	"fmt"
	"io"
)

// WriteRaw writes raw JSON bytes verbatim followed by a newline. Used
// for --json passthrough where re-serialization would drift from the
// upstream body (search items).
func WriteRaw(w io.Writer, raw []byte) error {
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("report: write json: %w", err)
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return fmt.Errorf("report: write json newline: %w", err)
	}
	return nil
}

// WriteIndent marshals v with two-space indentation and writes it plus a
// trailing newline. Used for --json of decoded structures (show/tags).
func WriteIndent(w io.Writer, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("report: encode json: %w", err)
	}
	return WriteRaw(w, b)
}
