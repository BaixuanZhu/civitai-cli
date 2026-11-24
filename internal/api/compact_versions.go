package api

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// CompactVersionSummary is the compact `civ show <modelId> -v <versionId>
// --json` projection. It keeps version identity, trigger words, every
// visible file's identity/hashes/scan facts, and example-meta coverage
// counts. It deliberately excludes description HTML, raw images, download
// URLs, ComfyUI/workflow payloads, training details, paid/early-access
// details, usage controls, and unknown fields; those remain available only
// through the legacy full view.
//
// Presence contract: every key is always present. Scalar members are null
// when the upstream value was absent or null; explicit zero/false/empty
// values are preserved. Array members are null when not provided and []
// when the source supplied an empty array; no array is truncated.
type CompactVersionSummary struct {
	// Identity fields. No latest-version or model-availability substitution
	// is performed.
	ID            *json.Number `json:"id"`
	ModelID       *json.Number `json:"modelId"`
	Name          *string      `json:"name"`
	BaseModel     *string      `json:"baseModel"`
	BaseModelType *string      `json:"baseModelType"`
	PublishedAt   *string      `json:"publishedAt"`
	Availability  *string      `json:"availability"`
	// Model is the owning model summary built from the standalone version's
	// own modelId and embedded model object; it never triggers a
	// /models/{id} request and never fabricates creator/license data.
	Model CompactOwningModel `json:"model"`
	// Stats is null when upstream stats were not provided.
	Stats *CompactVersionStats `json:"stats"`
	// TrainedWords preserves exact strings and order; null means the source
	// did not provide trigger words (not "no triggers").
	TrainedWords []string `json:"trainedWords"`
	// Files keeps every visible file in source order; null means not
	// provided, [] means supplied empty.
	Files []CompactFile `json:"files"`
	// Examples is null when images were not provided; otherwise it carries
	// counts only, never prompt text or image URLs.
	Examples *CompactExamples `json:"examples"`
}

// CompactOwningModel is the minimal owning-model identity of a version.
// ID comes from the version's top-level modelId; name and type come from
// the standalone version's embedded model object.
type CompactOwningModel struct {
	ID   *json.Number `json:"id"`
	Name *string      `json:"name"`
	Type *string      `json:"type"`
}

// CompactFile retains file identity, size, format/precision, scan results,
// and every supplied hash algorithm. DownloadURL is intentionally absent.
// All visible files are kept: non-primary Text Encoder/VAE files and their
// hashes/scans are part of version inspection.
type CompactFile struct {
	ID               *json.Number        `json:"id"`
	Name             *string             `json:"name"`
	SizeKB           *json.Number        `json:"sizeKB"`
	Type             *string             `json:"type"`
	Primary          *bool               `json:"primary"`
	Metadata         CompactFileMetadata `json:"metadata"`
	PickleScanResult *string             `json:"pickleScanResult"`
	VirusScanResult  *string             `json:"virusScanResult"`
	ScannedAt        *string             `json:"scannedAt"`
	// Hashes keeps every supplied algorithm/value pair; null means the
	// source omitted hashes, {} means the source supplied an empty object.
	Hashes map[string]string `json:"hashes"`
}

// CompactFileMetadata carries the file format/precision facts. The object
// is always emitted; members are null when not supplied.
type CompactFileMetadata struct {
	Format *string `json:"format"`
	FP     *string `json:"fp"`
	Size   *string `json:"size"`
}

// CompactExamples reports example-image metadata coverage. WithMetadata
// counts images whose meta is a non-null object (not the hasMeta flag);
// WithoutMetadata is Total minus WithMetadata. No prompt, URL, seed,
// dimension, or ComfyUI data is included.
type CompactExamples struct {
	Total           int `json:"total"`
	WithMetadata    int `json:"withMetadata"`
	WithoutMetadata int `json:"withoutMetadata"`
}

// ParseCompactVersion decodes a /model-versions/{id} response body into the
// compact version envelope. It projects the same response the legacy
// ParseVersion path uses and adds no request.
func ParseCompactVersion(raw []byte) (*CompactVersionEnvelope, error) {
	if err := compactRequireObject(raw); err != nil {
		return nil, err
	}
	var src compactRawVersion
	if err := json.Unmarshal(raw, &src); err != nil {
		return nil, fmt.Errorf("api: decode compact version: %w", err)
	}
	return &CompactVersionEnvelope{
		SchemaVersion: CompactShowSchemaVersion,
		Kind:          compactKindVersionSummary,
		Version:       compactProjectVersion(&src),
	}, nil
}

// compactRawVersion declares only the contracted version fields. The images
// are decoded as raw messages so metadata coverage can be counted without
// parsing generation payloads.
type compactRawVersion struct {
	ID            *json.Number            `json:"id"`
	ModelID       *json.Number            `json:"modelId"`
	Name          *string                 `json:"name"`
	BaseModel     *string                 `json:"baseModel"`
	BaseModelType *string                 `json:"baseModelType"`
	PublishedAt   *string                 `json:"publishedAt"`
	Availability  *string                 `json:"availability"`
	Model         *compactRawOwningModel  `json:"model"`
	Stats         *compactRawVersionStats `json:"stats"`
	TrainedWords  []string                `json:"trainedWords"`
	Files         []compactRawFile        `json:"files"`
	Images        []compactRawImage       `json:"images"`
}

// compactRawOwningModel is the embedded standalone model object; it carries
// no id (the owning id lives in compactRawVersion.ModelID).
type compactRawOwningModel struct {
	Name *string `json:"name"`
	Type *string `json:"type"`
}

// compactRawFile mirrors the contracted file fields; pointers preserve
// absent-versus-explicit values.
type compactRawFile struct {
	ID               *json.Number            `json:"id"`
	Name             *string                 `json:"name"`
	SizeKB           *json.Number            `json:"sizeKB"`
	Type             *string                 `json:"type"`
	Primary          *bool                   `json:"primary"`
	Metadata         *compactRawFileMetadata `json:"metadata"`
	PickleScanResult *string                 `json:"pickleScanResult"`
	VirusScanResult  *string                 `json:"virusScanResult"`
	ScannedAt        *string                 `json:"scannedAt"`
	Hashes           map[string]string       `json:"hashes"`
}

// compactRawFileMetadata is the file metadata source.
type compactRawFileMetadata struct {
	Format *string `json:"format"`
	FP     *string `json:"fp"`
	Size   *string `json:"size"`
}

// compactRawImage keeps only the metadata member needed for coverage counts.
// A RawMessage preserves null versus absent without decoding prompt content.
type compactRawImage struct {
	Meta json.RawMessage `json:"meta"`
}

// compactProjectVersion builds the versionSummary projection.
func compactProjectVersion(src *compactRawVersion) CompactVersionSummary {
	out := CompactVersionSummary{
		ID:            src.ID,
		ModelID:       src.ModelID,
		Name:          src.Name,
		BaseModel:     src.BaseModel,
		BaseModelType: src.BaseModelType,
		PublishedAt:   src.PublishedAt,
		Availability:  src.Availability,
		Model:         CompactOwningModel{ID: src.ModelID},
		TrainedWords:  src.TrainedWords,
	}
	if src.Model != nil {
		out.Model.Name = src.Model.Name
		out.Model.Type = src.Model.Type
	}
	if src.Stats != nil {
		out.Stats = compactProjectVersionStats(src.Stats)
	}
	if src.Files != nil {
		out.Files = make([]CompactFile, len(src.Files))
		for i := range src.Files {
			out.Files[i] = compactProjectFile(&src.Files[i])
		}
	}
	out.Examples = compactProjectExamples(src.Images)
	return out
}

// compactProjectFile builds one compact file entry. Metadata is always an
// object with nullable members so consumers never have to tell "missing
// metadata object" from "empty metadata" (the source semantics do not need
// that distinction); hashes preserve the null-versus-empty-object state.
func compactProjectFile(src *compactRawFile) CompactFile {
	out := CompactFile{
		ID:               src.ID,
		Name:             src.Name,
		SizeKB:           src.SizeKB,
		Type:             src.Type,
		Primary:          src.Primary,
		PickleScanResult: src.PickleScanResult,
		VirusScanResult:  src.VirusScanResult,
		ScannedAt:        src.ScannedAt,
		Hashes:           src.Hashes,
	}
	if src.Metadata != nil {
		out.Metadata = CompactFileMetadata{
			Format: src.Metadata.Format,
			FP:     src.Metadata.FP,
			Size:   src.Metadata.Size,
		}
	}
	return out
}

// compactProjectExamples counts image metadata coverage. It returns nil
// when the images member was absent or null, and preserves the explicit
// empty array as zero counts.
func compactProjectExamples(images []compactRawImage) *CompactExamples {
	if images == nil {
		return nil
	}
	out := &CompactExamples{Total: len(images)}
	for i := range images {
		if compactIsMetaObject(images[i].Meta) {
			out.WithMetadata++
		}
	}
	out.WithoutMetadata = out.Total - out.WithMetadata
	return out
}

// compactIsMetaObject reports whether a raw image meta value is a non-null
// JSON object. Null, absent, and non-object drift all count as no metadata.
func compactIsMetaObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '{'
}
