// Package resolver implements the pure evaluation core of `civ resolve`:
// approved reference validation, canonical identity correlation, reviewed
// compatibility rules, stable findings, and the fail-closed readiness
// verdict. It performs no HTTP, filesystem, cache, or printing work; those
// arrive through Environment function values, so the package is
// deterministic, offline-testable, and free of resource-management side
// effects. A nil error from Resolve means the evaluation completed (Ready
// may still be false); a non-nil error is a usage or operational failure
// and never carries a ready verdict.
package resolver

import (
	"context"
	"fmt"
)

// SchemaVersion is the version of the machine-readable report shape. It
// changes only when a consumer-visible JSON field changes incompatibly.
const SchemaVersion = 1

// RequestKind labels one of the four approved reference forms. No other
// form is accepted: bare integers, model ids, short hashes, and names are
// usage errors before any I/O.
type RequestKind string

const (
	// RequestPath is an explicit local regular-file path. Its content
	// hash supplies the registry identity; the path is the only form
	// that can prove local presence.
	RequestPath RequestKind = "path"
	// RequestSHA256 is a full 64-character SHA256 content reference. It
	// supplies canonical identity only and is locally unverified.
	RequestSHA256 RequestKind = "sha256"
	// RequestAirVersion is an AIR URN carrying an exact @<versionId>.
	// The numeric version id is authoritative; the URN is metadata.
	RequestAirVersion RequestKind = "air"
	// RequestVersionID is an exact numeric version reference
	// (version:<positiveId>) without AIR metadata.
	RequestVersionID RequestKind = "version"
)

// Request is one validated member of the declared resource set. Each ref
// makes exactly one identity claim; the approved CLI grammar never
// combines a path with a version or hash.
type Request struct {
	// InputIndex is the zero-based position supplied by the caller. Resolve
	// reports the slice position, so callers should keep these aligned.
	InputIndex int `json:"inputIndex"`
	// Kind is the approved reference form.
	Kind RequestKind `json:"kind"`
	// Ref is the original user-supplied reference text, retained for
	// stable reporting and evidence.
	Ref string `json:"ref"`
	// Path is the explicit local path for RequestPath.
	Path string `json:"path,omitempty"`
	// SHA256 is the normalized uppercase content hash for RequestSHA256.
	SHA256 string `json:"sha256,omitempty"`
	// VersionID is the exact numeric version for RequestAirVersion and
	// RequestVersionID.
	VersionID int `json:"versionId,omitempty"`
	// AIR is the source AIR URN for RequestAirVersion (display metadata).
	AIR string `json:"air,omitempty"`
	// ExpectedVersionID optionally binds a local path to an expected
	// canonical version. The approved CLI grammar exposes no path+version
	// association, so the command layer never sets it; the field exists
	// so the domain can report content/expected-version disagreement as
	// wrong_version (R5/AC5) instead of a generic failure.
	ExpectedVersionID int `json:"expectedVersionId,omitempty"`
}

// VersionMetadata is the resolver-owned projection of one registry
// version response. Numeric ids and SHA256 are identity; names, AIR, and
// base model fields are registry metadata carried for reporting and
// compatibility decisions. Missing values stay zero/empty — never guessed.
type VersionMetadata struct {
	ModelID       int            `json:"modelId"`
	VersionID     int            `json:"versionId"`
	AIR           string         `json:"air,omitempty"`
	Name          string         `json:"name,omitempty"`
	BaseModel     string         `json:"baseModel,omitempty"`
	BaseModelType string         `json:"baseModelType,omitempty"`
	ModelName     string         `json:"modelName,omitempty"`
	ModelType     string         `json:"modelType,omitempty"`
	Files         []FileMetadata `json:"files"`
}

// FileMetadata is one visible file of a version. SHA256 is only the
// explicitly projected hashes["SHA256"] value; other hash algorithms are
// not identity evidence for resolve.
type FileMetadata struct {
	FileID  int    `json:"fileId"`
	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	Primary bool   `json:"primary"`
	SHA256  string `json:"sha256,omitempty"`
}

// FileStatus is what an explicit path refers to, as observed by the
// environment. A missing path is a completed missing-resource finding;
// an existing non-regular target is a usage error.
type FileStatus struct {
	Exists  bool `json:"exists"`
	Regular bool `json:"regular"`
}

// Environment supplies the resolver's only I/O: explicit-path checks,
// streaming hashing, and read-only registry lookups. Function values keep
// the domain free of HTTP/filesystem imports and let tests run entirely
// in memory. Every field must be non-nil.
type Environment struct {
	// Stat reports whether path exists and is a regular file. A missing
	// path is (FileStatus{}, nil); other stat failures are operational.
	Stat func(path string) (FileStatus, error)
	// HashFile returns the uppercase SHA256 of the file at path.
	HashFile func(path string) (string, error)
	// VersionByID fetches one version by its exact numeric id.
	VersionByID func(ctx context.Context, id int) (VersionMetadata, error)
	// VersionByHash fetches the version containing a full SHA256.
	VersionByHash func(ctx context.Context, sha256 string) (VersionMetadata, error)
	// IsNotFound reports whether err is a registry 404. A 404 becomes a
	// completed unresolved_resource finding; every other error stays an
	// operational failure.
	IsNotFound func(err error) bool
}

// LocalState is the local-presence verdict of one resource.
type LocalState string

const (
	// LocalPresent means an explicit path was read, hashed, and matched
	// exactly one visible file of the intended registry version.
	LocalPresent LocalState = "present"
	// LocalMissing means the explicit path does not exist.
	LocalMissing LocalState = "missing"
	// LocalUnverified means no explicit path was supplied, so presence
	// cannot be established even when registry identity resolved.
	LocalUnverified LocalState = "unverified"
	// LocalUnresolved means local content identity could not be bound to
	// exactly one visible file, or the content is unregistered.
	LocalUnresolved LocalState = "unresolved"
	// LocalMismatch means the content resolved to a different canonical
	// version than the explicitly expected one.
	LocalMismatch LocalState = "mismatch"
)

// ResolutionState is the canonical-identity verdict of one resource.
type ResolutionState string

const (
	// ResolutionResolved means a canonical version identity was bound,
	// plus file identity when the request carried content.
	ResolutionResolved ResolutionState = "resolved"
	// ResolutionUnresolved means a well-formed reference could not be
	// resolved to canonical identity.
	ResolutionUnresolved ResolutionState = "unresolved"
	// ResolutionMismatch means content resolved to a version other than
	// the explicitly expected one.
	ResolutionMismatch ResolutionState = "mismatch"
	// ResolutionAmbiguousFile means more than one visible file carries the
	// same content hash, so no single file identity can be bound.
	ResolutionAmbiguousFile ResolutionState = "ambiguous_file"
)

// Identity is the canonical identity of a resolved resource. Model,
// version, and file ids and the exact SHA256 are authoritative; AIR and
// base metadata are registry metadata carried for reporting and
// compatibility.
type Identity struct {
	ModelID       int    `json:"modelId,omitempty"`
	VersionID     int    `json:"versionId,omitempty"`
	AIR           string `json:"air,omitempty"`
	FileID        int    `json:"fileId,omitempty"`
	SHA256        string `json:"sha256,omitempty"`
	ModelType     string `json:"modelType,omitempty"`
	BaseModel     string `json:"baseModel,omitempty"`
	BaseModelType string `json:"baseModelType,omitempty"`
}

// Names is display data only; it never establishes identity.
type Names struct {
	Model   string `json:"model,omitempty"`
	Version string `json:"version,omitempty"`
	File    string `json:"file,omitempty"`
}

// LocalEvidence is the local-presence record of one resource.
type LocalEvidence struct {
	State          LocalState `json:"state"`
	Path           string     `json:"path,omitempty"`
	ComputedSHA256 string     `json:"computedSha256,omitempty"`
	MatchedFileID  int        `json:"matchedFileId,omitempty"`
}

// Issue is the compact per-resource finding reference; the report's
// Findings array carries the full structured evidence.
type Issue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ResourceResult is one evaluated member of the declared set.
type ResourceResult struct {
	InputIndex   int             `json:"inputIndex"`
	RequestedRef string          `json:"requestedRef"`
	Kind         RequestKind     `json:"kind"`
	Resolution   ResolutionState `json:"resolution"`
	Identity     Identity        `json:"identity"`
	Names        Names           `json:"names"`
	VisibleFiles []FileMetadata  `json:"visibleFiles"`
	Local        LocalEvidence   `json:"local"`
	Issues       []Issue         `json:"issues"`
}

// Finding codes are the stable machine contract. Every finding is
// blocking in the initial policy: any one of them prevents ready=true.
const (
	// CodeMissingResource marks an explicitly declared path that does not
	// exist, or a reviewed required role that is absent.
	CodeMissingResource = "missing_resource"
	// CodeUnresolvedResource marks a well-formed registry reference that
	// cannot be resolved (including a registry 404).
	CodeUnresolvedResource = "unresolved_resource"
	// CodeWrongVersion marks content that resolves to a different
	// canonical version than the explicitly expected one.
	CodeWrongVersion = "wrong_version"
	// CodeLocalPresenceUnverified marks a remote-only reference whose
	// local presence cannot be proven; it can never contribute to ready.
	CodeLocalPresenceUnverified = "local_presence_unverified"
	// CodeLocalHashMismatch marks local content whose exact SHA256 matches
	// no visible file of the resolved version.
	CodeLocalHashMismatch = "local_hash_mismatch"
	// CodeUnresolvedFileIdentity marks a resolved version whose visible
	// files carry no SHA256 evidence able to bind the content.
	CodeUnresolvedFileIdentity = "unresolved_file_identity"
	// CodeAmbiguousFileIdentity marks content matching more than one
	// visible file; no file identity is guessed.
	CodeAmbiguousFileIdentity = "ambiguous_file_identity"
	// CodeMissingAnchor marks a set with no resolved Checkpoint resource.
	CodeMissingAnchor = "missing_anchor"
	// CodeConflictingAnchor marks a set with two or more distinct resolved
	// Checkpoint identities.
	CodeConflictingAnchor = "conflicting_anchor"
	// CodeUnknownCompatibility marks a tuple the reviewed policy cannot
	// prove compatible or incompatible.
	CodeUnknownCompatibility = "unknown_compatibility"
	// CodeBaseModelConflict marks a reviewed known conflict between the
	// anchor and a dependent resource.
	CodeBaseModelConflict = "base_model_conflict"
	// CodeRoleTypeMismatch is reserved for role-typed input; resolver-v1
	// derives roles from registry model.type only and never emits it.
	CodeRoleTypeMismatch = "role_type_mismatch"
)

// Finding is one stable, explainable evaluation result. Code is the
// machine contract; Evidence carries the observed expected/actual facts.
// Blocking is true for every finding in resolver-v1: each one prevents a
// ready verdict.
type Finding struct {
	Code            string   `json:"code"`
	Blocking        bool     `json:"blocking"`
	ResourceIndexes []int    `json:"resourceIndexes"`
	Message         string   `json:"message"`
	Evidence        Evidence `json:"evidence"`
	RuleID          string   `json:"ruleId,omitempty"`
}

// Evidence is the structured expected/actual evidence of a finding. Only
// the fields meaningful for the finding's code are populated.
type Evidence struct {
	Path      string      `json:"path,omitempty"`
	Reference string      `json:"reference,omitempty"`
	Expected  *Identity   `json:"expected,omitempty"`
	Actual    *Identity   `json:"actual,omitempty"`
	Left      *CompatSide `json:"left,omitempty"`
	Right     *CompatSide `json:"right,omitempty"`
}

// CompatSide is one side of a compatibility evaluation: the resource
// index plus the registry metadata the rule observed.
type CompatSide struct {
	Index         int    `json:"index"`
	ModelID       int    `json:"modelId,omitempty"`
	VersionID     int    `json:"versionId,omitempty"`
	ModelType     string `json:"modelType,omitempty"`
	BaseModel     string `json:"baseModel,omitempty"`
	BaseModelType string `json:"baseModelType,omitempty"`
}

// CompatibilityStatus is the outcome of one reviewed compatibility
// evaluation. Conflict (a reviewed known incompatibility) and unknown
// (unproven) are deliberately distinct; neither permits ready.
type CompatibilityStatus string

const (
	// StatusCompatible means the reviewed rule proved the tuple works.
	StatusCompatible CompatibilityStatus = "compatible"
	// StatusConflict means the reviewed rule proved a known conflict.
	StatusConflict CompatibilityStatus = "conflict"
	// StatusUnknown means the reviewed rules do not cover the tuple, or
	// required metadata is missing.
	StatusUnknown CompatibilityStatus = "unknown"
	// StatusNotEvaluated means the dependent's identity was unresolved, so
	// no compatibility metadata was available.
	StatusNotEvaluated CompatibilityStatus = "not_evaluated"
)

// CompatibilityResult is one evaluated anchor/dependent pair. Left is
// always the Checkpoint anchor; Right is the dependent resource.
type CompatibilityResult struct {
	LeftIndex  int                 `json:"leftIndex"`
	RightIndex int                 `json:"rightIndex"`
	Status     CompatibilityStatus `json:"status"`
	RuleID     string              `json:"ruleId"`
	Left       CompatSide          `json:"left"`
	Right      CompatSide          `json:"right"`
	Reason     string              `json:"reason,omitempty"`
}

// Summary is the aggregate counter block of a report.
type Summary struct {
	ResourceCount        int `json:"resourceCount"`
	BlockingFindingCount int `json:"blockingFindingCount"`
}

// Report is the complete versioned result of one resolve evaluation.
// Ordering is deterministic: resources follow input order, compatibility
// results follow input order, and findings follow evaluation order. No
// score, confidence, or percentage exists.
type Report struct {
	SchemaVersion int                   `json:"schemaVersion"`
	PolicyVersion string                `json:"policyVersion"`
	Ready         bool                  `json:"ready"`
	Resources     []ResourceResult      `json:"resources"`
	Compatibility []CompatibilityResult `json:"compatibility"`
	Findings      []Finding             `json:"findings"`
	Summary       Summary               `json:"summary"`
}

// Policy is the compiled, versioned review policy that decides the
// compatibility anchor, dependent roles, and base-model compatibility.
// It has no user-editable file or remote source: the binary ships exactly
// one reviewed policy.
type Policy struct {
	// Version is the stable policy identifier reported in every result.
	Version string
	// AnchorType is the registry model type of the single anchor role.
	AnchorType string
	// DependentTypes are the registry model types evaluated against the
	// anchor. Any other type is an uncovered tuple (unknown).
	DependentTypes []string
}

// UsageError marks malformed input or a non-regular explicit path: the
// caller must fix the command, no report is produced, and the process
// exits with the usage-class code.
type UsageError struct {
	// Message is the actionable usage problem.
	Message string
}

// Error returns the usage problem.
func (e *UsageError) Error() string { return e.Message }

// usageErrorf builds a UsageError with a formatted message.
func usageErrorf(format string, args ...any) error {
	return &UsageError{Message: fmt.Sprintf(format, args...)}
}
