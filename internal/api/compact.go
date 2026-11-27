package api

// Compact JSON projections for `civ search --json` and `civ show --json`.
//
// The compact views are the default machine contract; `--view full` keeps
// the legacy outputs (raw search items, decoded Model/ModelVersion) for
// migration. The projections therefore live beside the legacy decoders
// (ParseModel, ParseVersion, ModelsRawItems) instead of changing them:
// unrelated commands and the full view must not drift.
//
// Every compact field is part of the approved field registry. Unknown
// upstream fields are tolerated by the decoder but never emitted, and no
// projection adds a request, a fallback, or a dynamic field selector.

import (
	"bytes"
	"fmt"
)

const (
	// CompactSearchSchemaVersion versions the compact `civ search --json`
	// envelope. Removing or changing a contracted field requires a new
	// schema version and a release-note review.
	CompactSearchSchemaVersion = "civ.search/v1"

	// CompactShowSchemaVersion versions the compact `civ show --json`
	// envelopes (model summary and version summary).
	CompactShowSchemaVersion = "civ.show/v1"

	// compactKindModelSearch and compactKindModelSummary are the envelope
	// `kind` discriminators. The search envelope carries model summaries,
	// so it is distinguished from a single model summary by kind.
	compactKindModelSearch  = "modelSearch"
	compactKindModelSummary = "modelSummary"

	// compactKindVersionSummary is the `kind` of the version-show envelope.
	compactKindVersionSummary = "versionSummary"
)

// CompactSearchEnvelope is the compact `civ search --json` shape: the
// versioned envelope replaces the legacy raw items array. Items keep the
// upstream length and order; an explicit empty upstream items array
// serializes as []. Summary is an additive v1 projection over those same
// items: it describes only the returned page and never a match total.
type CompactSearchEnvelope struct {
	SchemaVersion string                `json:"schemaVersion"`
	Kind          string                `json:"kind"`
	Items         []CompactModelSummary `json:"items"`
	Summary       SearchResultSummary   `json:"summary"`
}

// SearchBaseCoverage counts how many returned models observed one base
// model name. Models is a coverage count — at most one contribution per
// model — not an occurrence count, so a multi-base model increments every
// observed base and the coverage values need not sum to Returned. Only
// observed nonzero categories appear; BaseModel is the exact upstream
// string, never trimmed, case-folded, or checked against the base filter
// whitelist.
type SearchBaseCoverage struct {
	BaseModel string `json:"baseModel"`
	Models    int    `json:"models"`
}

// SearchResultSummary aggregates the base-model evidence of the models in
// one search response. Returned is exactly the response item count (zero
// for an empty page), never a total derived from cursors or metadata.
// BaseCoverage is always present and serializes as [] when no base was
// observed. ModelsWithMultipleBases counts models whose selected evidence
// set holds more than one distinct base; ModelsWithoutBase counts models
// with no usable evidence. No percentage, total, cursor, or primary-base
// field exists on purpose: coverage categories are not exclusive buckets.
type SearchResultSummary struct {
	Returned                int                  `json:"returned"`
	BaseCoverage            []SearchBaseCoverage `json:"baseCoverage"`
	ModelsWithMultipleBases int                  `json:"modelsWithMultipleBases"`
	ModelsWithoutBase       int                  `json:"modelsWithoutBase"`
}

// CompactModelEnvelope is the compact `civ show <modelId> --json` shape.
// Model is the same modelSummary object used by compact search items.
type CompactModelEnvelope struct {
	SchemaVersion string              `json:"schemaVersion"`
	Kind          string              `json:"kind"`
	Model         CompactModelSummary `json:"model"`
}

// CompactVersionEnvelope is the compact `civ show <modelId> -v <versionId>
// --json` shape.
type CompactVersionEnvelope struct {
	SchemaVersion string                `json:"schemaVersion"`
	Kind          string                `json:"kind"`
	Version       CompactVersionSummary `json:"version"`
}

// compactRequireObject rejects a response whose root is not a JSON object.
// A null/array/scalar root is not a model, version, or search envelope, so
// it must fail instead of silently projecting zero values.
func compactRequireObject(raw []byte) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("api: compact response root is not a JSON object")
	}
	return nil
}
