// Package compare builds the pure, deterministic model/version comparison
// result shared by the human and JSON renderers. It performs no HTTP,
// printing, file access, clock, random, or process-exit work: callers hand
// it presence-aware API snapshots and receive one Result.
//
// The package owns the public comparison contract: the ordered field
// registries, the missing/null/empty value states, the confirmed-equality
// rules, and the baseline-to-each-later-subject derived operations. The
// first subject is the directional baseline only; it is never a ranking or
// recommendation.
package compare

import (
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"civitai-cli/internal/api"
)

// SchemaVersion is the released machine-contract version. It covers the
// value-state semantics, field-key meaning, and derived-operation shapes;
// later incompatible changes require a bump.
const SchemaVersion = 1

// Kind is the comparison subject family.
type Kind string

const (
	// KindModel compares model-level facts.
	KindModel Kind = "model"
	// KindVersion compares explicitly selected model versions.
	KindVersion Kind = "version"
)

// State is the presence state of one projected value.
type State string

const (
	// StateValue means the source supplied a non-null value, including
	// false, 0, "", [], and {}.
	StateValue State = "value"
	// StateNull means the source supplied an explicit JSON null.
	StateNull State = "null"
	// StateMissing means the source did not supply the field.
	StateMissing State = "missing"
)

// Comparison is the confirmed equality state of one aligned field.
type Comparison string

const (
	// ComparisonEqual means every subject supplied an equal value.
	ComparisonEqual Comparison = "equal"
	// ComparisonDifferent means at least two supplied values differ (even
	// when another subject is missing).
	ComparisonDifferent Comparison = "different"
	// ComparisonIndeterminate means equality cannot be confirmed, either
	// because every value is missing or because equal known values are
	// accompanied by missing ones.
	ComparisonIndeterminate Comparison = "indeterminate"
)

// Derived operation states. Derived operations run only when every operand
// is supplied and non-null; otherwise they report indeterminate instead of
// empty change arrays that would read as "no change".
const (
	// DerivedApplied means every operand was available.
	DerivedApplied = "applied"
	// DerivedIndeterminate means at least one operand was missing or null.
	DerivedIndeterminate = "indeterminate"
)

// Value sources. The source is per value, not per field, because the same
// field can come from different responses across subjects.
const (
	// SourceVersion is the standalone /model-versions/{id} response.
	SourceVersion = "version"
	// SourceModel is the owning /models/{id} response.
	SourceModel = "model"
	// SourceEmbeddedVersion is the matching entry inside the owning model's
	// modelVersions array (availability/status fallback).
	SourceEmbeddedVersion = "embeddedVersion"
	// SourceDerived is a value computed by this package (URLs, summaries).
	SourceDerived = "derived"
)

// Value is one subject's presence-aware value for a registry field.
type Value struct {
	State  State  `json:"state"`
	Source string `json:"source"`
	// Value is the typed curated value when State is value, otherwise null.
	Value any `json:"value"`
}

// Subject identifies one comparison subject by input position.
type Subject struct {
	Index     int    `json:"index"`
	ModelID   int64  `json:"modelId"`
	VersionID int64  `json:"versionId,omitempty"`
	Name      string `json:"name"`
	URL       string `json:"url"`
}

// Field is one aligned registry fact: one value position per subject, a
// completeness/equality summary, and an optional derived operation.
// ValueType is one of string, bool, integer, number, stringSet, fileList,
// or recipeCoverage.
type Field struct {
	Key        string     `json:"key"`
	ValueType  string     `json:"valueType"`
	Values     []Value    `json:"values"`
	Complete   bool       `json:"complete"`
	Comparison Comparison `json:"comparison"`
	// Derived is nil for identity fields and serializes as null when no
	// derivation applies.
	Derived any `json:"derived"`
}

// Result is the single versioned machine contract and the only input to
// both human and JSON rendering.
type Result struct {
	SchemaVersion int       `json:"schemaVersion"`
	Kind          Kind      `json:"kind"`
	Baseline      int       `json:"baseline"`
	Subjects      []Subject `json:"subjects"`
	Fields        []Field   `json:"fields"`
}

// ModelSubject is one selected model: the requested ID plus its decoded
// comparison snapshot.
type ModelSubject struct {
	ID       int64
	Snapshot *api.CompareModel
}

// VersionSubject is one selected version plus its owning-model snapshot.
// Parent is required by the command's strict fetch policy; a nil parent is
// tolerated here and simply projects model-level facts as missing.
type VersionSubject struct {
	ID       int64
	Snapshot *api.CompareVersion
	Parent   *api.CompareModel
}

// FileValue is one curated file inside a subject's files value. Every
// scalar keeps a presence state so absent metadata is not compared as an
// empty string or false; hashes are sorted by algorithm, and files appear
// in canonical order (registry ID, then exact name, then type).
type FileValue struct {
	ID               Optional `json:"id"`
	Name             Optional `json:"name"`
	Type             Optional `json:"type"`
	Primary          Optional `json:"primary"`
	SizeKB           Optional `json:"sizeKB"`
	Format           Optional `json:"format"`
	FP               Optional `json:"fp"`
	Size             Optional `json:"size"`
	PickleScanResult Optional `json:"pickleScanResult"`
	VirusScanResult  Optional `json:"virusScanResult"`
	ScannedAt        Optional `json:"scannedAt"`
	// HashesState distinguishes absent/null hashes from a supplied empty
	// object; Hashes holds the supplied entries in algorithm order.
	HashesState State       `json:"hashesState"`
	Hashes      []HashEntry `json:"hashes"`
}

// Optional is one presence-aware file attribute. State is value, null, or
// missing; Value carries the typed attribute only when State is value.
type Optional struct {
	State State
	Value any
}

// MarshalJSON renders an Optional compactly: missing/null keep only the
// state, while a supplied value always carries an explicit value field
// (including false, 0, and "").
func (o Optional) MarshalJSON() ([]byte, error) {
	if o.State != StateValue {
		return json.Marshal(struct {
			State State `json:"state"`
		}{State: o.State})
	}
	return json.Marshal(struct {
		State State `json:"state"`
		Value any   `json:"value"`
	}{State: o.State, Value: o.Value})
}

// HashEntry is one supplied file hash algorithm/value pair.
type HashEntry struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

// RecipeCoverage is the projected example-image recipe coverage of one
// version. Counts are observations, not quality judgments.
type RecipeCoverage struct {
	ImagesTotal         int            `json:"imagesTotal"`
	HasMetaTrue         int            `json:"hasMetaTrue"`
	HasMetaFalse        int            `json:"hasMetaFalse"`
	HasMetaNull         int            `json:"hasMetaNull"`
	HasMetaMissing      int            `json:"hasMetaMissing"`
	MetaPresent         int            `json:"metaPresent"`
	DeclaredMetaMissing int            `json:"declaredMetaMissing"`
	NonEmpty            NonEmptyCounts `json:"nonEmpty"`
}

// NonEmptyCounts counts metadata objects that supplied a non-empty value
// for each supported recipe field. It is coverage, not a quality score.
type NonEmptyCounts struct {
	Prompt         int `json:"prompt"`
	NegativePrompt int `json:"negativePrompt"`
	Steps          int `json:"steps"`
	CFGScale       int `json:"cfgScale"`
	Sampler        int `json:"sampler"`
	Scheduler      int `json:"scheduler"`
	Seed           int `json:"seed"`
	Width          int `json:"width"`
	Height         int `json:"height"`
}

// Registry value types carried by Field.ValueType.
const (
	valueTypeString         = "string"
	valueTypeBool           = "bool"
	valueTypeInteger        = "integer"
	valueTypeNumber         = "number"
	valueTypeStringSet      = "stringSet"
	valueTypeFileList       = "fileList"
	valueTypeRecipeCoverage = "recipeCoverage"
)

// BuildModels projects model snapshots in input order into a comparison
// Result. The first subject is the baseline for derived operations.
func BuildModels(subjects []ModelSubject) *Result {
	res := &Result{
		SchemaVersion: SchemaVersion,
		Kind:          KindModel,
		Baseline:      0,
		Subjects:      make([]Subject, 0, len(subjects)),
	}
	for i, s := range subjects {
		name := ""
		if s.Snapshot != nil && s.Snapshot.Name.HasValue() {
			name = s.Snapshot.Name.Value
		}
		res.Subjects = append(res.Subjects, Subject{
			Index:   i,
			ModelID: s.ID,
			Name:    name,
			URL:     modelURL(s.ID),
		})
	}
	res.Fields = buildFields(modelRegistry, func(i int) projection {
		return projection{subjectID: subjects[i].ID, model: subjects[i].Snapshot}
	}, len(subjects))
	return res
}

// BuildVersions projects version snapshots (plus their owning models) in
// input order into a comparison Result. The first subject is the baseline.
func BuildVersions(subjects []VersionSubject) *Result {
	res := &Result{
		SchemaVersion: SchemaVersion,
		Kind:          KindVersion,
		Baseline:      0,
		Subjects:      make([]Subject, 0, len(subjects)),
	}
	for i, s := range subjects {
		name := ""
		if s.Snapshot != nil && s.Snapshot.Name.HasValue() {
			name = s.Snapshot.Name.Value
		}
		ownerID := versionOwnerID(s)
		res.Subjects = append(res.Subjects, Subject{
			Index:     i,
			ModelID:   ownerID,
			VersionID: s.ID,
			Name:      name,
			URL:       versionURL(ownerID, s.ID),
		})
	}
	res.Fields = buildFields(versionRegistry, func(i int) projection {
		return projection{
			subjectID: subjects[i].ID,
			version:   subjects[i].Snapshot,
			parent:    subjects[i].Parent,
		}
	}, len(subjects))
	return res
}

// IsIdentityField reports whether key is a subject identity fact for kind:
// the selected IDs and the URLs derived from them. Identity fields are
// always different across distinct selections, so the diff view suppresses
// them while the full view and subject headers still show them.
func IsIdentityField(kind Kind, key string) bool {
	switch kind {
	case KindModel:
		return key == "modelId" || key == "url"
	case KindVersion:
		return key == "versionId" || key == "modelId" || key == "modelURL"
	}
	return false
}

// projection is one subject's snapshots as seen by a registry entry.
type projection struct {
	subjectID int64
	model     *api.CompareModel
	version   *api.CompareVersion
	parent    *api.CompareModel
}

// fieldSpec is one registry entry: public key, value type, projection, and
// optional derived operation (nil for identity fields).
type fieldSpec struct {
	key       string
	valueType string
	project   func(projection) Value
	derive    func([]Value) any
}

// buildFields evaluates an ordered registry for n subjects, aligning
// values[i] with subjects[i] and deriving baseline-to-each-later changes.
func buildFields(registry []fieldSpec, project func(int) projection, n int) []Field {
	fields := make([]Field, 0, len(registry))
	for _, spec := range registry {
		values := make([]Value, n)
		for i := 0; i < n; i++ {
			values[i] = spec.project(project(i))
		}
		complete, comparison := compareStates(values)
		f := Field{
			Key:        spec.key,
			ValueType:  spec.valueType,
			Values:     values,
			Complete:   complete,
			Comparison: comparison,
		}
		if spec.derive != nil {
			f.Derived = spec.derive(values)
		}
		fields = append(fields, f)
	}
	return fields
}

// compareStates applies the approved completeness/equality rules: at least
// two supplied values that differ make the field different even when
// another value is missing; equal known values with a missing one are
// indeterminate; all-missing is indeterminate.
func compareStates(values []Value) (bool, Comparison) {
	complete := true
	known := make([]Value, 0, len(values))
	for _, v := range values {
		if v.State == StateMissing {
			complete = false
			continue
		}
		known = append(known, v)
	}
	if len(known) == 0 {
		return false, ComparisonIndeterminate
	}
	equal := true
	for _, v := range known[1:] {
		if !sameValue(known[0], v) {
			equal = false
			break
		}
	}
	switch {
	case !equal:
		return complete, ComparisonDifferent
	case !complete:
		return false, ComparisonIndeterminate
	default:
		return true, ComparisonEqual
	}
}

// sameValue compares two supplied values; states are part of the fact, so
// null, [], and "" are distinct. File lists compare with known hexadecimal
// hash values normalized case-insensitively, matching the derived hash
// rules.
func sameValue(a, b Value) bool {
	if a.State != b.State {
		return false
	}
	if a.State != StateValue {
		return true
	}
	if af, ok := a.Value.([]FileValue); ok {
		bf, ok := b.Value.([]FileValue)
		if !ok {
			return false
		}
		return sameFileValues(af, bf)
	}
	return reflect.DeepEqual(a.Value, b.Value)
}

// sameFileValues compares canonical file lists with normalized hash values.
func sameFileValues(a, b []FileValue) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !reflect.DeepEqual(normalizeHashes(a[i]), normalizeHashes(b[i])) {
			return false
		}
	}
	return true
}

// normalizeHashes returns a copy of f whose known hexadecimal hash values
// are uppercased for equality; the original aligned facts keep their exact
// source text.
func normalizeHashes(f FileValue) FileValue {
	out := f
	out.Hashes = append([]HashEntry{}, f.Hashes...)
	for i := range out.Hashes {
		h := &out.Hashes[i]
		if isKnownHexHashAlgorithm(h.Algorithm) && isHexString(h.Value) {
			h.Value = strings.ToUpper(h.Value)
		}
	}
	return out
}

// fromOpt converts one API presence-aware scalar to a Result value.
func fromOpt[T any](o api.Opt[T], source string) Value {
	switch o.Presence {
	case api.PresenceValue:
		return Value{State: StateValue, Source: source, Value: o.Value}
	case api.PresenceNull:
		return Value{State: StateNull, Source: source}
	default:
		return Value{State: StateMissing, Source: source}
	}
}

// fromSet converts a presence-aware string list to a deduplicated,
// lexicographically sorted set value; supplied order and duplicates never
// create a difference.
func fromSet(o api.Opt[[]string], source string) Value {
	if !o.HasValue() {
		return fromOpt(o, source)
	}
	return Value{State: StateValue, Source: source, Value: sortedUnique(o.Value)}
}

// sortedUnique returns a non-nil deduplicated lexical copy of in.
func sortedUnique(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	uniq := out[:0]
	for _, s := range out {
		if len(uniq) == 0 || uniq[len(uniq)-1] != s {
			uniq = append(uniq, s)
		}
	}
	if uniq == nil {
		uniq = []string{}
	}
	return uniq
}

// modelURL builds the canonical public model URL, or missing when the id
// is unusable.
func modelURL(id int64) string {
	if id <= 0 {
		return ""
	}
	return "https://civitai.com/models/" + strconv.FormatInt(id, 10)
}

// versionURL builds the canonical public model-version URL, or an empty
// string when the owning id is unusable.
func versionURL(modelID, versionID int64) string {
	if modelID <= 0 {
		return ""
	}
	if versionID <= 0 {
		return modelURL(modelID)
	}
	return "https://civitai.com/models/" + strconv.FormatInt(modelID, 10) +
		"?modelVersionId=" + strconv.FormatInt(versionID, 10)
}

// versionOwnerID resolves the owning model id from standalone detail, then
// the parent snapshot.
func versionOwnerID(s VersionSubject) int64 {
	if s.Snapshot != nil && s.Snapshot.ModelID.HasValue() && s.Snapshot.ModelID.Value > 0 {
		return s.Snapshot.ModelID.Value
	}
	if s.Parent != nil && s.Parent.ID.HasValue() && s.Parent.ID.Value > 0 {
		return s.Parent.ID.Value
	}
	return 0
}

// modelRegistry is the approved ordered model-comparison field set.
var modelRegistry = []fieldSpec{
	{key: "modelId", valueType: valueTypeInteger, project: func(p projection) Value {
		if p.subjectID <= 0 {
			return missingValue(SourceModel)
		}
		return Value{State: StateValue, Source: SourceModel, Value: p.subjectID}
	}},
	{key: "name", valueType: valueTypeString, project: func(p projection) Value {
		return modelOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.Name, SourceModel) })
	}, derive: deriveScalar},
	{key: "url", valueType: valueTypeString, project: func(p projection) Value {
		u := modelURL(p.subjectID)
		if u == "" {
			return missingValue(SourceDerived)
		}
		return Value{State: StateValue, Source: SourceDerived, Value: u}
	}},
	{key: "creator", valueType: valueTypeString, project: func(p projection) Value {
		return modelOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.Creator, SourceModel) })
	}, derive: deriveScalar},
	{key: "type", valueType: valueTypeString, project: func(p projection) Value {
		return modelOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.Type, SourceModel) })
	}, derive: deriveScalar},
	{key: "baseModels", valueType: valueTypeStringSet, project: func(p projection) Value {
		return modelOpt(p, func(m *api.CompareModel) Value { return fromSet(m.BaseModels, SourceModel) })
	}, derive: deriveSet},
	{key: "tags", valueType: valueTypeStringSet, project: func(p projection) Value {
		return modelOpt(p, func(m *api.CompareModel) Value { return fromSet(m.Tags, SourceModel) })
	}, derive: deriveSet},
	{key: "nsfw", valueType: valueTypeBool, project: func(p projection) Value {
		return modelOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.NSFW, SourceModel) })
	}, derive: deriveScalar},
	{key: "nsfwLevel", valueType: valueTypeNumber, project: func(p projection) Value {
		return modelOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.NSFWLevel, SourceModel) })
	}, derive: deriveScalar},
	{key: "availability", valueType: valueTypeString, project: func(p projection) Value {
		return modelOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.Availability, SourceModel) })
	}, derive: deriveScalar},
	{key: "allowNoCredit", valueType: valueTypeBool, project: func(p projection) Value {
		return modelOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.AllowNoCredit, SourceModel) })
	}, derive: deriveScalar},
	{key: "allowCommercialUse", valueType: valueTypeStringSet, project: func(p projection) Value {
		return modelOpt(p, func(m *api.CompareModel) Value { return fromSet(m.AllowCommercialUse, SourceModel) })
	}, derive: deriveSet},
	{key: "allowDerivatives", valueType: valueTypeBool, project: func(p projection) Value {
		return modelOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.AllowDerivatives, SourceModel) })
	}, derive: deriveScalar},
	{key: "allowDifferentLicense", valueType: valueTypeBool, project: func(p projection) Value {
		return modelOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.AllowDifferentLicense, SourceModel) })
	}, derive: deriveScalar},
	{key: "downloadCount", valueType: valueTypeInteger, project: func(p projection) Value {
		return modelOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.Downloads, SourceModel) })
	}, derive: deriveInt},
	{key: "thumbsUpCount", valueType: valueTypeInteger, project: func(p projection) Value {
		return modelOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.ThumbsUp, SourceModel) })
	}, derive: deriveInt},
	{key: "thumbsDownCount", valueType: valueTypeInteger, project: func(p projection) Value {
		return modelOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.ThumbsDown, SourceModel) })
	}, derive: deriveInt},
	{key: "commentCount", valueType: valueTypeInteger, project: func(p projection) Value {
		return modelOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.Comments, SourceModel) })
	}, derive: deriveInt},
	{key: "tippedAmountCount", valueType: valueTypeInteger, project: func(p projection) Value {
		return modelOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.Tips, SourceModel) })
	}, derive: deriveInt},
	{key: "versionCount", valueType: valueTypeInteger, project: func(p projection) Value {
		if p.model == nil || !p.model.Versions.HasValue() {
			if p.model != nil && p.model.Versions.Presence == api.PresenceNull {
				return Value{State: StateNull, Source: SourceModel}
			}
			return missingValue(SourceModel)
		}
		return Value{State: StateValue, Source: SourceModel, Value: int64(len(p.model.Versions.Value))}
	}, derive: deriveInt},
	{key: "latestPublishedAt", valueType: valueTypeString, project: func(p projection) Value {
		return latestPublishedAt(p.model)
	}, derive: deriveScalar},
}

// versionRegistry is the approved ordered version-comparison field set.
var versionRegistry = []fieldSpec{
	{key: "versionId", valueType: valueTypeInteger, project: func(p projection) Value {
		if p.subjectID <= 0 {
			return missingValue(SourceVersion)
		}
		return Value{State: StateValue, Source: SourceVersion, Value: p.subjectID}
	}},
	{key: "versionName", valueType: valueTypeString, project: func(p projection) Value {
		return versionOpt(p, func(v *api.CompareVersion) Value { return fromOpt(v.Name, SourceVersion) })
	}, derive: deriveScalar},
	{key: "modelId", valueType: valueTypeInteger, project: func(p projection) Value {
		if p.version != nil && p.version.ModelID.HasValue() && p.version.ModelID.Value > 0 {
			return Value{State: StateValue, Source: SourceVersion, Value: p.version.ModelID.Value}
		}
		if p.parent != nil && p.parent.ID.HasValue() && p.parent.ID.Value > 0 {
			return Value{State: StateValue, Source: SourceModel, Value: p.parent.ID.Value}
		}
		return missingValue(SourceVersion)
	}},
	{key: "modelName", valueType: valueTypeString, project: func(p projection) Value {
		return parentOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.Name, SourceModel) })
	}, derive: deriveScalar},
	{key: "modelURL", valueType: valueTypeString, project: func(p projection) Value {
		id := versionOwnerID(VersionSubject{Snapshot: p.version, Parent: p.parent})
		u := modelURL(id)
		if u == "" {
			return missingValue(SourceDerived)
		}
		return Value{State: StateValue, Source: SourceDerived, Value: u}
	}},
	{key: "modelType", valueType: valueTypeString, project: func(p projection) Value {
		return parentOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.Type, SourceModel) })
	}, derive: deriveScalar},
	{key: "creator", valueType: valueTypeString, project: func(p projection) Value {
		return parentOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.Creator, SourceModel) })
	}, derive: deriveScalar},
	{key: "baseModel", valueType: valueTypeString, project: func(p projection) Value {
		return versionOpt(p, func(v *api.CompareVersion) Value { return fromOpt(v.BaseModel, SourceVersion) })
	}, derive: deriveScalar},
	{key: "baseModelType", valueType: valueTypeString, project: func(p projection) Value {
		return versionOpt(p, func(v *api.CompareVersion) Value { return fromOpt(v.BaseModelType, SourceVersion) })
	}, derive: deriveScalar},
	{key: "publishedAt", valueType: valueTypeString, project: func(p projection) Value {
		return versionOpt(p, func(v *api.CompareVersion) Value { return fromOpt(v.PublishedAt, SourceVersion) })
	}, derive: deriveScalar},
	{key: "status", valueType: valueTypeString, project: func(p projection) Value {
		return versionWithEmbeddedFallback(p, func(v *api.CompareVersion) api.Opt[string] { return v.Status },
			func(e *api.CompareEmbeddedVersion) api.Opt[string] { return e.Status })
	}, derive: deriveScalar},
	{key: "availability", valueType: valueTypeString, project: func(p projection) Value {
		return versionWithEmbeddedFallback(p, func(v *api.CompareVersion) api.Opt[string] { return v.Availability },
			func(e *api.CompareEmbeddedVersion) api.Opt[string] { return e.Availability })
	}, derive: deriveScalar},
	{key: "nsfwLevel", valueType: valueTypeNumber, project: func(p projection) Value {
		return versionOpt(p, func(v *api.CompareVersion) Value { return fromOpt(v.NSFWLevel, SourceVersion) })
	}, derive: deriveScalar},
	{key: "allowNoCredit", valueType: valueTypeBool, project: func(p projection) Value {
		return parentOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.AllowNoCredit, SourceModel) })
	}, derive: deriveScalar},
	{key: "allowCommercialUse", valueType: valueTypeStringSet, project: func(p projection) Value {
		return parentOpt(p, func(m *api.CompareModel) Value { return fromSet(m.AllowCommercialUse, SourceModel) })
	}, derive: deriveSet},
	{key: "allowDerivatives", valueType: valueTypeBool, project: func(p projection) Value {
		return parentOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.AllowDerivatives, SourceModel) })
	}, derive: deriveScalar},
	{key: "allowDifferentLicense", valueType: valueTypeBool, project: func(p projection) Value {
		return parentOpt(p, func(m *api.CompareModel) Value { return fromOpt(m.AllowDifferentLicense, SourceModel) })
	}, derive: deriveScalar},
	{key: "downloadCount", valueType: valueTypeInteger, project: func(p projection) Value {
		return versionOpt(p, func(v *api.CompareVersion) Value { return fromOpt(v.Downloads, SourceVersion) })
	}, derive: deriveInt},
	{key: "thumbsUpCount", valueType: valueTypeInteger, project: func(p projection) Value {
		return versionOpt(p, func(v *api.CompareVersion) Value { return fromOpt(v.ThumbsUp, SourceVersion) })
	}, derive: deriveInt},
	{key: "thumbsDownCount", valueType: valueTypeInteger, project: func(p projection) Value {
		return versionOpt(p, func(v *api.CompareVersion) Value { return fromOpt(v.ThumbsDown, SourceVersion) })
	}, derive: deriveInt},
	{key: "trainedWords", valueType: valueTypeStringSet, project: func(p projection) Value {
		return versionOpt(p, func(v *api.CompareVersion) Value { return fromSet(v.TrainedWords, SourceVersion) })
	}, derive: deriveSet},
	{key: "files", valueType: valueTypeFileList, project: func(p projection) Value {
		if p.version == nil || !p.version.Files.HasValue() {
			if p.version != nil && p.version.Files.Presence == api.PresenceNull {
				return Value{State: StateNull, Source: SourceVersion}
			}
			return missingValue(SourceVersion)
		}
		return Value{State: StateValue, Source: SourceVersion, Value: toFileValues(p.version.Files.Value)}
	}, derive: deriveFiles},
	{key: "recipeCoverage", valueType: valueTypeRecipeCoverage, project: func(p projection) Value {
		if p.version == nil || !p.version.Images.HasValue() {
			if p.version != nil && p.version.Images.Presence == api.PresenceNull {
				return Value{State: StateNull, Source: SourceVersion}
			}
			return missingValue(SourceVersion)
		}
		return Value{State: StateValue, Source: SourceVersion, Value: projectRecipeCoverage(p.version.Images.Value)}
	}, derive: deriveRecipe},
}

// missingValue builds a missing value with its attempted source.
func missingValue(source string) Value {
	return Value{State: StateMissing, Source: source}
}

// modelOpt projects a model-sourced value, degrading a nil snapshot to
// missing.
func modelOpt(p projection, project func(*api.CompareModel) Value) Value {
	if p.model == nil {
		return missingValue(SourceModel)
	}
	return project(p.model)
}

// versionOpt projects a standalone-version value, degrading a nil snapshot
// to missing.
func versionOpt(p projection, project func(*api.CompareVersion) Value) Value {
	if p.version == nil {
		return missingValue(SourceVersion)
	}
	return project(p.version)
}

// parentOpt projects an owning-model value, degrading a nil snapshot to
// missing.
func parentOpt(p projection, project func(*api.CompareModel) Value) Value {
	if p.parent == nil {
		return missingValue(SourceModel)
	}
	return project(p.parent)
}

// versionWithEmbeddedFallback projects status/availability: standalone
// detail stays authoritative, and only a missing field falls back to the
// matching embedded version. Null is supplied, so it never falls back.
func versionWithEmbeddedFallback(p projection, fromVersion func(*api.CompareVersion) api.Opt[string], fromEmbedded func(*api.CompareEmbeddedVersion) api.Opt[string]) Value {
	if p.version != nil {
		o := fromVersion(p.version)
		switch o.Presence {
		case api.PresenceValue:
			return Value{State: StateValue, Source: SourceVersion, Value: o.Value}
		case api.PresenceNull:
			return Value{State: StateNull, Source: SourceVersion}
		}
	}
	if ev := findEmbedded(p.parent, p.subjectID); ev != nil {
		o := fromEmbedded(ev)
		switch o.Presence {
		case api.PresenceValue:
			return Value{State: StateValue, Source: SourceEmbeddedVersion, Value: o.Value}
		case api.PresenceNull:
			return Value{State: StateNull, Source: SourceEmbeddedVersion}
		}
	}
	return missingValue(SourceVersion)
}

// findEmbedded locates the embedded version whose id matches versionID.
func findEmbedded(parent *api.CompareModel, versionID int64) *api.CompareEmbeddedVersion {
	if parent == nil || versionID <= 0 || !parent.Versions.HasValue() {
		return nil
	}
	for i := range parent.Versions.Value {
		ev := &parent.Versions.Value[i]
		if ev.ID.HasValue() && ev.ID.Value == versionID {
			return ev
		}
	}
	return nil
}

// latestPublishedAt derives the latest embedded-version publication as a
// normalized UTC RFC3339 timestamp. Missing/null/unparseable timestamps or
// an empty version list make the summary missing rather than a zero date or
// a confirmed latest value.
func latestPublishedAt(m *api.CompareModel) Value {
	if m == nil || !m.Versions.HasValue() || len(m.Versions.Value) == 0 {
		return missingValue(SourceDerived)
	}
	var latest time.Time
	for _, ev := range m.Versions.Value {
		if !ev.PublishedAt.HasValue() {
			return missingValue(SourceDerived)
		}
		ts, err := time.Parse(time.RFC3339, ev.PublishedAt.Value)
		if err != nil {
			return missingValue(SourceDerived)
		}
		if latest.IsZero() || ts.After(latest) {
			latest = ts
		}
	}
	if latest.IsZero() {
		return missingValue(SourceDerived)
	}
	return Value{State: StateValue, Source: SourceDerived, Value: latest.UTC().Format(time.RFC3339Nano)}
}

// toFileValues converts API file snapshots to canonical file values. The
// canonical order (ID, then name, then type) makes API ordering alone
// unable to create a difference.
func toFileValues(in []api.CompareFile) []FileValue {
	out := make([]FileValue, 0, len(in))
	for _, f := range in {
		fv := FileValue{
			ID:               optional(f.ID),
			Name:             optional(f.Name),
			Type:             optional(f.Type),
			Primary:          optional(f.Primary),
			SizeKB:           optional(f.SizeKB),
			Format:           optional(f.Format),
			FP:               optional(f.FP),
			Size:             optional(f.Size),
			PickleScanResult: optional(f.PickleScanResult),
			VirusScanResult:  optional(f.VirusScanResult),
			ScannedAt:        optional(f.ScannedAt),
			HashesState:      StateMissing,
		}
		switch f.Hashes.Presence {
		case api.PresenceValue:
			fv.HashesState = StateValue
			fv.Hashes = make([]HashEntry, 0, len(f.Hashes.Value))
			for _, h := range f.Hashes.Value {
				fv.Hashes = append(fv.Hashes, HashEntry{Algorithm: h.Algorithm, Value: h.Value})
			}
		case api.PresenceNull:
			fv.HashesState = StateNull
		}
		if fv.Hashes == nil {
			fv.Hashes = []HashEntry{}
		}
		out = append(out, fv)
	}
	return canonicalFiles(out)
}

// optional converts one API presence-aware attribute to a file Optional.
func optional[T any](o api.Opt[T]) Optional {
	switch o.Presence {
	case api.PresenceValue:
		return Optional{State: StateValue, Value: o.Value}
	case api.PresenceNull:
		return Optional{State: StateNull}
	default:
		return Optional{State: StateMissing}
	}
}

// projectRecipeCoverage counts recipe coverage over one supplied images
// array. Counts are computed independently: a declared-but-null metadata
// object is not an empty recipe, and an absent hasMeta is not false.
func projectRecipeCoverage(images []api.CompareImage) RecipeCoverage {
	var cov RecipeCoverage
	cov.ImagesTotal = len(images)
	for _, im := range images {
		switch im.HasMeta.Presence {
		case api.PresenceValue:
			if im.HasMeta.Value {
				cov.HasMetaTrue++
			} else {
				cov.HasMetaFalse++
			}
		case api.PresenceNull:
			cov.HasMetaNull++
		default:
			cov.HasMetaMissing++
		}
		if im.Meta.HasValue() {
			cov.MetaPresent++
			meta := im.Meta.Value
			if meta.Prompt {
				cov.NonEmpty.Prompt++
			}
			if meta.NegativePrompt {
				cov.NonEmpty.NegativePrompt++
			}
			if meta.Steps {
				cov.NonEmpty.Steps++
			}
			if meta.CFGScale {
				cov.NonEmpty.CFGScale++
			}
			if meta.Sampler {
				cov.NonEmpty.Sampler++
			}
			if meta.Scheduler {
				cov.NonEmpty.Scheduler++
			}
			if meta.Seed {
				cov.NonEmpty.Seed++
			}
			if meta.Width {
				cov.NonEmpty.Width++
			}
			if meta.Height {
				cov.NonEmpty.Height++
			}
		}
		if im.HasMeta.HasValue() && im.HasMeta.Value && !im.Meta.HasValue() {
			cov.DeclaredMetaMissing++
		}
	}
	return cov
}
