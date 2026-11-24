package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Presence classifies how an upstream JSON field was supplied. It is the
// comparison-layer vocabulary for the PRD's missing/null/empty contract:
// a missing key is not false, zero, or an empty list, and an explicit null
// is distinct from both. The zero value is PresenceMissing so an
// undecoded field cannot masquerade as supplied.
type Presence int

const (
	// PresenceMissing means the response did not supply the field.
	PresenceMissing Presence = iota
	// PresenceValue means the response supplied a non-null value, including
	// false, 0, "", [], and {}.
	PresenceValue
	// PresenceNull means the response supplied an explicit JSON null.
	PresenceNull
)

// String renders the presence state for diagnostics and test failures.
func (p Presence) String() string {
	switch p {
	case PresenceValue:
		return "value"
	case PresenceNull:
		return "null"
	default:
		return "missing"
	}
}

// Opt is one presence-aware decoded value. Presence carries the missing/
// null/value state and Value is meaningful only when Presence is
// PresenceValue. A supplied but unusable (wrong JSON shape) value decodes
// as PresenceMissing so comparison stays indeterminate instead of
// fabricating a typed value.
type Opt[T any] struct {
	Presence Presence
	Value    T
}

// HasValue reports whether the field was supplied as a non-null value.
func (o Opt[T]) HasValue() bool { return o.Presence == PresenceValue }

// CompareModel is the presence-aware comparison snapshot of GET
// /models/{id}. It keeps only the curated comparison fields; descriptions,
// cover images, download URLs, and unknown objects are dropped at this
// boundary. Existing Model decoding and its JSON shape are untouched.
type CompareModel struct {
	ID                    Opt[int64]
	Name                  Opt[string]
	Type                  Opt[string]
	NSFW                  Opt[bool]
	NSFWLevel             Opt[any] // int64 bitmask or preserved enum text
	Tags                  Opt[[]string]
	BaseModels            Opt[[]string]
	Availability          Opt[string]
	AllowNoCredit         Opt[bool]
	AllowCommercialUse    Opt[[]string]
	AllowDerivatives      Opt[bool]
	AllowDifferentLicense Opt[bool]
	Creator               Opt[string]
	Downloads             Opt[int64]
	ThumbsUp              Opt[int64]
	ThumbsDown            Opt[int64]
	Comments              Opt[int64]
	Tips                  Opt[int64]
	// Versions is the embedded model-versions list reduced to the facts
	// comparison needs: identity, availability/status fallback for stand-
	// alone version detail, and publishedAt for the latest-publication
	// summary. Missing versus an explicitly empty list is preserved.
	Versions Opt[[]CompareEmbeddedVersion]
}

// CompareEmbeddedVersion is one embedded model-versions entry reduced to
// the comparison facts. Availability and Status are retained so a version
// comparison can fall back to the matching embedded entry when standalone
// detail omits them.
type CompareEmbeddedVersion struct {
	ID           Opt[int64]
	Availability Opt[string]
	Status       Opt[string]
	PublishedAt  Opt[string]
}

// CompareVersion is the presence-aware comparison snapshot of GET
// /model-versions/{id}. Prompts, image URLs, download URLs, and unknown
// objects are dropped here; example images are reduced to recipe-presence
// facts so coverage counts can be derived without retaining payload text.
type CompareVersion struct {
	ID            Opt[int64]
	ModelID       Opt[int64]
	Name          Opt[string]
	BaseModel     Opt[string]
	BaseModelType Opt[string]
	PublishedAt   Opt[string]
	Status        Opt[string]
	Availability  Opt[string]
	NSFWLevel     Opt[any]
	TrainedWords  Opt[[]string]
	Downloads     Opt[int64]
	ThumbsUp      Opt[int64]
	ThumbsDown    Opt[int64]
	Files         Opt[[]CompareFile]
	Images        Opt[[]CompareImage]
}

// CompareFile is one version file reduced to curated comparison fields.
// Every scalar keeps its presence so missing metadata is not compared as
// an empty string or false. Hashes preserves every supplied algorithm in a
// deterministic algorithm-name order; download URLs are excluded.
type CompareFile struct {
	ID               Opt[int64]
	Name             Opt[string]
	Type             Opt[string]
	Primary          Opt[bool]
	SizeKB           Opt[float64]
	Format           Opt[string]
	FP               Opt[string]
	Size             Opt[string]
	PickleScanResult Opt[string]
	VirusScanResult  Opt[string]
	ScannedAt        Opt[string]
	Hashes           Opt[[]CompareHash]
}

// CompareHash is one supplied file hash algorithm/value pair.
type CompareHash struct {
	Algorithm string
	Value     string
}

// CompareImage is one embedded example image reduced to recipe-coverage
// facts. HasMeta keeps the absent/false/true distinction, and Meta keeps
// absent versus explicit null versus a supplied metadata object.
type CompareImage struct {
	HasMeta Opt[bool]
	Meta    Opt[CompareMeta]
}

// CompareMeta records which supported recipe fields were supplied with a
// non-empty value. Raw prompt text and the values themselves are never
// retained; width/height count as non-empty for any supplied non-null JSON
// value, including ComfyUI node objects.
type CompareMeta struct {
	Prompt         bool
	NegativePrompt bool
	Steps          bool
	CFGScale       bool
	Sampler        bool
	Scheduler      bool
	Seed           bool
	Width          bool
	Height         bool
}

// ParseCompareModel decodes a /models/{id} response body into a comparison
// snapshot. Unknown fields are ignored; malformed JSON returns a wrapped
// decode error. A successful decode may still carry missing fields — that
// is data, not an error.
func ParseCompareModel(raw []byte) (*CompareModel, error) {
	m, err := decodeRawObject(raw)
	if err != nil {
		return nil, fmt.Errorf("api: decode compare model: %w", err)
	}
	cm := &CompareModel{
		ID:                    optValue(m, "id", decodeInt64),
		Name:                  optValue(m, "name", decodeString),
		Type:                  optValue(m, "type", decodeString),
		NSFW:                  optValue(m, "nsfw", decodeBool),
		NSFWLevel:             optValue(m, "nsfwLevel", decodeNSFWLevel),
		Tags:                  optValue(m, "tags", decodeStrings),
		BaseModels:            optValue(m, "baseModels", decodeStrings),
		Availability:          optValue(m, "availability", decodeString),
		AllowNoCredit:         optValue(m, "allowNoCredit", decodeBool),
		AllowCommercialUse:    optValue(m, "allowCommercialUse", decodeStrings),
		AllowDerivatives:      optValue(m, "allowDerivatives", decodeBool),
		AllowDifferentLicense: optValue(m, "allowDifferentLicense", decodeBool),
		Creator:               optValue(m, "creator", decodeCreator),
		Versions:              parseEmbeddedVersions(m),
	}
	downloads, up, down, comments, tips := parseStats(m)
	cm.Downloads, cm.ThumbsUp, cm.ThumbsDown, cm.Comments, cm.Tips = downloads, up, down, comments, tips
	return cm, nil
}

// ParseCompareVersion decodes a /model-versions/{id} body into a comparison
// snapshot. Unknown fields are ignored; malformed JSON returns a wrapped
// decode error. Missing versus null versus supplied-empty values survive
// for every curated field.
func ParseCompareVersion(raw []byte) (*CompareVersion, error) {
	m, err := decodeRawObject(raw)
	if err != nil {
		return nil, fmt.Errorf("api: decode compare version: %w", err)
	}
	cv := &CompareVersion{
		ID:            optValue(m, "id", decodeInt64),
		ModelID:       optValue(m, "modelId", decodeInt64),
		Name:          optValue(m, "name", decodeString),
		BaseModel:     optValue(m, "baseModel", decodeString),
		BaseModelType: optValue(m, "baseModelType", decodeString),
		PublishedAt:   optValue(m, "publishedAt", decodeString),
		Status:        optValue(m, "status", decodeString),
		Availability:  optValue(m, "availability", decodeString),
		NSFWLevel:     optValue(m, "nsfwLevel", decodeNSFWLevel),
		TrainedWords:  optValue(m, "trainedWords", decodeStrings),
		Files:         parseFiles(m),
		Images:        parseImages(m),
	}
	downloads, up, down, _, _ := parseStats(m)
	cv.Downloads, cv.ThumbsUp, cv.ThumbsDown = downloads, up, down
	return cv, nil
}

// decodeRawObject decodes one JSON object into its raw field map. The map
// is used only to establish key presence; every value is decoded through
// the typed helpers below.
func decodeRawObject(raw []byte) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// fieldPresence splits a raw field into its presence state. json.RawMessage
// preserves the value bytes; only the literal null (after whitespace
// trimming) counts as an explicit null.
func fieldPresence(m map[string]json.RawMessage, key string) (json.RawMessage, Presence) {
	raw, ok := m[key]
	if !ok {
		return nil, PresenceMissing
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return raw, PresenceNull
	}
	return raw, PresenceValue
}

// optValue decodes one field through decode when it is supplied non-null.
// An undecodable shape degrades to missing rather than a fabricated value.
func optValue[T any](m map[string]json.RawMessage, key string, decode func(json.RawMessage) (T, bool)) Opt[T] {
	raw, presence := fieldPresence(m, key)
	switch presence {
	case PresenceMissing:
		return Opt[T]{Presence: PresenceMissing}
	case PresenceNull:
		return Opt[T]{Presence: PresenceNull}
	}
	v, ok := decode(raw)
	if !ok {
		return Opt[T]{Presence: PresenceMissing}
	}
	return Opt[T]{Presence: PresenceValue, Value: v}
}

// decodeString accepts a JSON string.
func decodeString(raw json.RawMessage) (string, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// decodeBool accepts a JSON boolean only; numeric 0/1 drift is not treated
// as a boolean fact.
func decodeBool(raw json.RawMessage) (bool, bool) {
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return false, false
	}
	return b, true
}

// decodeInt64 accepts a JSON number or a numeric string, tolerating
// non-fractional float forms (66832.0) and string drift ("66832").
func decodeInt64(raw json.RawMessage) (int64, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return parseIntText(s)
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, false
	}
	if i, err := n.Int64(); err == nil {
		return i, true
	}
	f, err := n.Float64()
	if err != nil || f != math.Trunc(f) || f < math.MinInt64 || f > math.MaxInt64 {
		return 0, false
	}
	return int64(f), true
}

// parseIntText parses a JSON numeric string, accepting integral float text
// such as "66832.0".
func parseIntText(s string) (int64, bool) {
	if i, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
		return i, true
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f != math.Trunc(f) || f < math.MinInt64 || f > math.MaxInt64 {
		return 0, false
	}
	return int64(f), true
}

// decodeFloat64 accepts a JSON number or a numeric string.
func decodeFloat64(raw json.RawMessage) (float64, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		return f, err == nil
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, false
	}
	f, err := n.Float64()
	return f, err == nil
}

// decodeStrings accepts a JSON array of strings. An empty array yields a
// non-nil empty slice so "supplied empty" stays distinct from missing.
func decodeStrings(raw json.RawMessage) ([]string, bool) {
	var s []string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, false
	}
	if s == nil {
		s = []string{}
	}
	return s, true
}

// decodeCreator extracts the creator username from the creator object. A
// creator object without a supplied username degrades to missing so an
// absent username is not compared as an empty creator.
func decodeCreator(raw json.RawMessage) (string, bool) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", false
	}
	u := optValue(m, "username", decodeString)
	if !u.HasValue() {
		return "", false
	}
	return u.Value, true
}

// decodeNSFWLevel handles the models-endpoint numeric bitmask and string
// drift: integral numbers and numeric strings normalize to int64; unknown
// enum names are preserved verbatim as strings.
func decodeNSFWLevel(raw json.RawMessage) (any, bool) {
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		if i, err := n.Int64(); err == nil {
			return i, true
		}
		return n.String(), true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, false
	}
	if i, ok := parseIntText(s); ok {
		return i, true
	}
	return s, true
}

// parseStats decodes the five engagement counters. An absent or non-object
// stats block leaves every counter missing.
func parseStats(m map[string]json.RawMessage) (downloads, up, down, comments, tips Opt[int64]) {
	raw, presence := fieldPresence(m, "stats")
	if presence != PresenceValue {
		return Opt[int64]{Presence: presence}, Opt[int64]{Presence: presence},
			Opt[int64]{Presence: presence}, Opt[int64]{Presence: presence},
			Opt[int64]{Presence: presence}
	}
	var sm map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sm); err != nil {
		return Opt[int64]{Presence: PresenceMissing}, Opt[int64]{Presence: PresenceMissing},
			Opt[int64]{Presence: PresenceMissing}, Opt[int64]{Presence: PresenceMissing},
			Opt[int64]{Presence: PresenceMissing}
	}
	return optValue(sm, "downloadCount", decodeInt64),
		optValue(sm, "thumbsUpCount", decodeInt64),
		optValue(sm, "thumbsDownCount", decodeInt64),
		optValue(sm, "commentCount", decodeInt64),
		optValue(sm, "tippedAmountCount", decodeInt64)
}

// parseEmbeddedVersions reduces the modelVersions array to comparison
// facts. A missing/null array stays missing so version-count summaries are
// not fabricated from absent data.
func parseEmbeddedVersions(m map[string]json.RawMessage) Opt[[]CompareEmbeddedVersion] {
	raw, presence := fieldPresence(m, "modelVersions")
	if presence != PresenceValue {
		return Opt[[]CompareEmbeddedVersion]{Presence: presence}
	}
	var list []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return Opt[[]CompareEmbeddedVersion]{Presence: PresenceMissing}
	}
	out := make([]CompareEmbeddedVersion, 0, len(list))
	for _, vm := range list {
		out = append(out, CompareEmbeddedVersion{
			ID:           optValue(vm, "id", decodeInt64),
			Availability: optValue(vm, "availability", decodeString),
			Status:       optValue(vm, "status", decodeString),
			PublishedAt:  optValue(vm, "publishedAt", decodeString),
		})
	}
	return Opt[[]CompareEmbeddedVersion]{Presence: PresenceValue, Value: out}
}

// parseFiles reduces the files array to curated file snapshots.
func parseFiles(m map[string]json.RawMessage) Opt[[]CompareFile] {
	raw, presence := fieldPresence(m, "files")
	if presence != PresenceValue {
		return Opt[[]CompareFile]{Presence: presence}
	}
	var list []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return Opt[[]CompareFile]{Presence: PresenceMissing}
	}
	out := make([]CompareFile, 0, len(list))
	for _, fm := range list {
		f := CompareFile{
			ID:               optValue(fm, "id", decodeInt64),
			Name:             optValue(fm, "name", decodeString),
			Type:             optValue(fm, "type", decodeString),
			Primary:          optValue(fm, "primary", decodeBool),
			SizeKB:           optValue(fm, "sizeKB", decodeFloat64),
			PickleScanResult: optValue(fm, "pickleScanResult", decodeString),
			VirusScanResult:  optValue(fm, "virusScanResult", decodeString),
			ScannedAt:        optValue(fm, "scannedAt", decodeString),
			Hashes:           parseHashes(fm),
		}
		if mr, mp := fieldPresence(fm, "metadata"); mp == PresenceValue {
			var mm map[string]json.RawMessage
			if json.Unmarshal(mr, &mm) == nil {
				f.Format = optValue(mm, "format", decodeString)
				f.FP = optValue(mm, "fp", decodeString)
				f.Size = optValue(mm, "size", decodeString)
			}
		}
		out = append(out, f)
	}
	return Opt[[]CompareFile]{Presence: PresenceValue, Value: out}
}

// parseHashes decodes the hashes object into algorithm-sorted entries.
// Map iteration order never reaches the output.
func parseHashes(fm map[string]json.RawMessage) Opt[[]CompareHash] {
	raw, presence := fieldPresence(fm, "hashes")
	if presence != PresenceValue {
		return Opt[[]CompareHash]{Presence: presence}
	}
	var hm map[string]json.RawMessage
	if err := json.Unmarshal(raw, &hm); err != nil {
		return Opt[[]CompareHash]{Presence: PresenceMissing}
	}
	out := make([]CompareHash, 0, len(hm))
	for algo, rawVal := range hm {
		val, ok := decodeString(rawVal)
		if !ok {
			continue
		}
		out = append(out, CompareHash{Algorithm: algo, Value: val})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Algorithm < out[j].Algorithm })
	return Opt[[]CompareHash]{Presence: PresenceValue, Value: out}
}

// parseImages reduces example images to recipe-coverage facts.
func parseImages(m map[string]json.RawMessage) Opt[[]CompareImage] {
	raw, presence := fieldPresence(m, "images")
	if presence != PresenceValue {
		return Opt[[]CompareImage]{Presence: presence}
	}
	var list []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return Opt[[]CompareImage]{Presence: PresenceMissing}
	}
	out := make([]CompareImage, 0, len(list))
	for _, im := range list {
		ci := CompareImage{HasMeta: optValue(im, "hasMeta", decodeBool)}
		if mr, mp := fieldPresence(im, "meta"); mp == PresenceValue {
			var mm map[string]json.RawMessage
			if json.Unmarshal(mr, &mm) == nil {
				ci.Meta = Opt[CompareMeta]{Presence: PresenceValue, Value: metaPresence(mm)}
			}
		} else if mp == PresenceNull {
			ci.Meta = Opt[CompareMeta]{Presence: PresenceNull}
		}
		out = append(out, ci)
	}
	return Opt[[]CompareImage]{Presence: PresenceValue, Value: out}
}

// metaPresence records which supported recipe fields are supplied with a
// non-empty value: strings must be non-empty, numeric parameters must be
// usable numbers, and width/height count for any non-null JSON value.
func metaPresence(m map[string]json.RawMessage) CompareMeta {
	return CompareMeta{
		Prompt:         nonEmptyString(m, "prompt"),
		NegativePrompt: nonEmptyString(m, "negativePrompt"),
		Steps:          suppliedNumber(m, "steps"),
		CFGScale:       suppliedNumber(m, "cfgScale"),
		Sampler:        nonEmptyString(m, "sampler"),
		Scheduler:      nonEmptyString(m, "scheduler"),
		Seed:           suppliedNumber(m, "seed"),
		Width:          suppliedNonNull(m, "width"),
		Height:         suppliedNonNull(m, "height"),
	}
}

// nonEmptyString reports whether the field is a supplied non-empty string.
func nonEmptyString(m map[string]json.RawMessage, key string) bool {
	o := optValue(m, key, decodeString)
	return o.HasValue() && o.Value != ""
}

// suppliedNumber reports whether the field is a supplied numeric value
// (JSON number or numeric string).
func suppliedNumber(m map[string]json.RawMessage, key string) bool {
	raw, presence := fieldPresence(m, key)
	if presence != PresenceValue {
		return false
	}
	_, ok := decodeFloat64(raw)
	return ok
}

// suppliedNonNull reports whether the field is supplied with any non-null
// JSON value (including objects and arrays).
func suppliedNonNull(m map[string]json.RawMessage, key string) bool {
	_, presence := fieldPresence(m, key)
	return presence == PresenceValue
}
