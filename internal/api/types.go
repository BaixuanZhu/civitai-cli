// Package api is the single outbound choke point for the Civitai REST API
// (https://civitai.com/api/v1). It owns the HTTP client (retry, concurrency
// gate, auth, body limit), typed error classification, tolerant response
// structs, and the CLI→API enum mapping tables. Endpoint methods return
// raw response bodies; decoding helpers (ParseModels, ParseModel,
// ParseVersion, ParseTags) are separate so --json can pass raw bytes
// through without re-serialization drift.
//
// All schema knowledge here is empirical (verified live 2026-11-22, see
// testdata/FIXTURES.md): the API has no version promise, so decoding is
// deliberately tolerant — unknown fields are ignored, unknown enum values
// are preserved as-is, and numeric fields accept string drift.
package api

import (
	"encoding/json"
	"fmt"
)

// Model is a /models item or /models/{id} response. NSFWLevel is the
// numeric bitmask form used by the models endpoint family (1=None 2=Soft
// 4=Mature 8=X, unknown bits render as L<bit>); the images browse endpoint
// uses a string form instead and is intentionally not consumed here.
type Model struct {
	ID                    int            `json:"id"`
	Name                  string         `json:"name"`
	Type                  string         `json:"type"` // unknown enum values preserved as-is
	NSFW                  bool           `json:"nsfw"`
	NSFWLevel             json.Number    `json:"nsfwLevel"` // numeric bitmask; string drift tolerated
	Description           string         `json:"description"`
	Tags                  []string       `json:"tags"`
	BaseModels            []string       `json:"baseModels"`
	AllowNoCredit         bool           `json:"allowNoCredit"`
	AllowCommercialUse    []string       `json:"allowCommercialUse"`
	AllowDerivatives      bool           `json:"allowDerivatives"`
	AllowDifferentLicense bool           `json:"allowDifferentLicense"`
	Availability          string         `json:"availability"`
	Stats                 Stats          `json:"stats"` // zero-value safe when absent/{}
	Creator               Creator        `json:"creator"`
	ModelVersions         []ModelVersion `json:"modelVersions"`
}

// UnmarshalJSON decodes a Model tolerantly: nsfwLevel accepts both the
// numeric bitmask and a string ("7" or a drifted enum name like "Soft"),
// keeping the raw text in the json.Number field for the render layer.
func (m *Model) UnmarshalJSON(b []byte) error {
	type alias Model
	aux := struct {
		*alias
		NSFWLevel json.RawMessage `json:"nsfwLevel"`
	}{alias: (*alias)(m)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	m.NSFWLevel = flexibleNumber(aux.NSFWLevel)
	return nil
}

// ModelVersion is a version object: embedded in Model.ModelVersions, or
// standalone from /model-versions/{id} and /model-versions/by-hash/{hash}.
// Embedded versions may omit TrainedWords (nil means "not provided",
// distinct from an empty slice) and Model; standalone versions carry both.
type ModelVersion struct {
	ID            int           `json:"id"`
	ModelID       int           `json:"modelId"` // owning model id (standalone form)
	Name          string        `json:"name"`
	BaseModel     string        `json:"baseModel"`
	BaseModelType string        `json:"baseModelType"`
	PublishedAt   string        `json:"publishedAt"` // ISO8601; render layer takes YYYY-MM-DD
	TrainedWords  []string      `json:"trainedWords"`
	Availability  string        `json:"availability"`
	NSFWLevel     json.Number   `json:"nsfwLevel"`
	Stats         Stats         `json:"stats"`
	DownloadURL   string        `json:"downloadUrl"`
	Files         []ModelFile   `json:"files"`
	Images        []Image       `json:"images"`
	Model         *ModelSummary `json:"model"` // owning model summary (standalone form)
	// AIR is the canonical AIR identifier of this version as returned by
	// the registry (e.g. urn:air:anima:checkpoint:civitai:2458426@3263843).
	// It is auxiliary metadata for resolve reporting, not a file hash or a
	// substitute for numeric ids, so JSON output must never gain an "air"
	// key: the field is tag-excluded and populated only by UnmarshalJSON.
	AIR string `json:"-"`
}

// UnmarshalJSON decodes a ModelVersion with the same tolerant nsfwLevel
// handling as Model, plus the auxiliary air field (tolerantly absent: many
// embedded versions omit it).
func (v *ModelVersion) UnmarshalJSON(b []byte) error {
	type alias ModelVersion
	aux := struct {
		*alias
		NSFWLevel json.RawMessage `json:"nsfwLevel"`
		AIR       string          `json:"air"`
	}{alias: (*alias)(v)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	v.NSFWLevel = flexibleNumber(aux.NSFWLevel)
	v.AIR = aux.AIR
	return nil
}

// ModelSummary is the owning-model summary embedded in standalone version
// responses. Verified live 2026-11-22: it carries {name,type,nsfw,poi}
// (no id — the owning id is ModelVersion.ModelID).
type ModelSummary struct {
	Name string `json:"name"`
	Type string `json:"type"`
	NSFW bool   `json:"nsfw"`
}

// ModelFile is one downloadable file of a version. Hashes keeps every
// hash algorithm the API returns (SHA256, AutoV1, AutoV2, AutoV3, BLAKE3,
// CRC32, SHA256_12, ...) without cherry-picking.
type ModelFile struct {
	ID               int               `json:"id"`
	Name             string            `json:"name"`
	SizeKB           float64           `json:"sizeKB"`
	Type             string            `json:"type"` // Model / VAE / ... unknown values preserved
	Primary          bool              `json:"primary"`
	PickleScanResult string            `json:"pickleScanResult"`
	VirusScanResult  string            `json:"virusScanResult"`
	ScannedAt        string            `json:"scannedAt"`
	Metadata         FileMetadata      `json:"metadata"`
	Hashes           map[string]string `json:"hashes"`
	DownloadURL      string            `json:"downloadUrl"`
}

// FileMetadata is the format/precision metadata of a model file.
type FileMetadata struct {
	Format string `json:"format"` // SafeTensor / PickleTensor / ...
	FP     string `json:"fp"`     // bf16 / fp16 / fp32 / ...
	Size   string `json:"size"`   // optional; absent in most live payloads
}

// Image is an example image embedded in a version. Meta is a pointer
// because the API sends null (or omits it) even when HasMeta is true —
// verified live 2026-11-22. NSFWLevel here is the numeric form.
type Image struct {
	URL       string      `json:"url"`
	NSFWLevel json.Number `json:"nsfwLevel"`
	HasMeta   bool        `json:"hasMeta"`
	Meta      *ImageMeta  `json:"meta"` // null-safe
}

// UnmarshalJSON decodes an Image with tolerant nsfwLevel handling.
func (im *Image) UnmarshalJSON(b []byte) error {
	type alias Image
	aux := struct {
		*alias
		NSFWLevel json.RawMessage `json:"nsfwLevel"`
	}{alias: (*alias)(im)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	im.NSFWLevel = flexibleNumber(aux.NSFWLevel)
	return nil
}

// ImageMeta is the generation recipe of an example image. Numeric fields
// use json.Number to survive string/number drift; Width/Height use
// json.RawMessage because ComfyUI uploads embed node objects there
// (verified live 2026-11-22 on version 3263843). Unknown fields (comfy,
// engine, models, resources, ...) are ignored until a command needs them.
type ImageMeta struct {
	Prompt         string          `json:"prompt"`
	NegativePrompt string          `json:"negativePrompt"`
	Steps          json.Number     `json:"steps"`
	CFGScale       json.Number     `json:"cfgScale"`
	Sampler        string          `json:"sampler"`
	Scheduler      string          `json:"scheduler"`
	Seed           json.Number     `json:"seed"`
	Width          json.RawMessage `json:"width"`
	Height         json.RawMessage `json:"height"`
}

// Stats is the engagement counters block. It is a value struct so an
// absent or {} stats object decodes to safe zeros (design D4).
type Stats struct {
	DownloadCount     int64 `json:"downloadCount"`
	ThumbsUpCount     int64 `json:"thumbsUpCount"`
	ThumbsDownCount   int64 `json:"thumbsDownCount"`
	CommentCount      int64 `json:"commentCount"`
	TippedAmountCount int64 `json:"tippedAmountCount"`
}

// Creator is the model author block; only the username is consumed in M1.
type Creator struct {
	Username string `json:"username"`
}

// Tag is one /tags completion item.
type Tag struct {
	Name string `json:"name"`
	Link string `json:"link"`
}

// flexibleNumber normalizes a raw JSON value into a json.Number: numbers
// keep their literal text; strings keep their content (numeric strings
// like "7" render as bitmasks, enum names like "Soft" render as-is).
// Absent/null yields "" which the render layer treats as missing.
func flexibleNumber(raw json.RawMessage) json.Number {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return json.Number(s)
	}
	return json.Number(raw)
}

// ParseModels decodes the items array of a /models response body.
func ParseModels(raw []byte) ([]Model, error) {
	var env struct {
		Items []Model `json:"items"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("api: decode models: %w", err)
	}
	return env.Items, nil
}

// ModelsRawItems extracts the verbatim items-array bytes from a /models
// response body for --json passthrough: no decode/re-encode round trip,
// so unknown fields and original formatting survive untouched.
func ModelsRawItems(raw []byte) (json.RawMessage, error) {
	var env struct {
		Items json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("api: decode models envelope: %w", err)
	}
	if len(env.Items) == 0 {
		return nil, fmt.Errorf("api: response has no items array")
	}
	return env.Items, nil
}

// ParseModel decodes a /models/{id} response body.
func ParseModel(raw []byte) (*Model, error) {
	var m Model
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("api: decode model: %w", err)
	}
	return &m, nil
}

// ParseVersion decodes a /model-versions/{id} or by-hash response body.
func ParseVersion(raw []byte) (*ModelVersion, error) {
	var v ModelVersion
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("api: decode version: %w", err)
	}
	return &v, nil
}

// ParseTags decodes the items array of a /tags response body.
func ParseTags(raw []byte) ([]Tag, error) {
	var env struct {
		Items []Tag `json:"items"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("api: decode tags: %w", err)
	}
	return env.Items, nil
}
