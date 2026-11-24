package resolver

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ParseRef validates one CLI reference and returns its request form. It is
// a pure syntactic check: no file or network access happens here, so
// unaccepted forms (bare ids, model ids, names, short hashes, AIR without
// an exact version) fail as usage errors before any I/O (AC6).
func ParseRef(index int, ref string) (Request, error) {
	if strings.TrimSpace(ref) == "" {
		return Request{}, usageErrorf("empty resource reference")
	}
	if strings.HasPrefix(ref, "version:") {
		id, err := parsePositiveID(ref[len("version:"):])
		if err != nil {
			return Request{}, usageErrorf("%q is not a valid version reference; want version:<positiveId>", ref)
		}
		return Request{InputIndex: index, Kind: RequestVersionID, Ref: ref, VersionID: id}, nil
	}
	if len(ref) == 64 && isHexString(ref) {
		return Request{InputIndex: index, Kind: RequestSHA256, Ref: ref, SHA256: strings.ToUpper(ref)}, nil
	}
	if hasAIRPrefix(ref) {
		id, err := parseAIRVersion(ref)
		if err != nil {
			return Request{}, err
		}
		return Request{InputIndex: index, Kind: RequestAirVersion, Ref: ref, VersionID: id, AIR: ref}, nil
	}
	if strings.HasPrefix(strings.ToLower(ref), "urn:") {
		return Request{}, usageErrorf("%q is not an AIR URN with an exact @<versionId>", ref)
	}
	if isAllDigits(ref) {
		return Request{}, usageErrorf("%q looks like a bare numeric id; use version:<id>, a full SHA256, an AIR URN with @<versionId>, or an explicit path", ref)
	}
	if isHexString(ref) {
		return Request{}, usageErrorf("%q looks like a short hash; resolve requires a full 64-character SHA256", ref)
	}
	if !LooksLikePath(ref) {
		return Request{}, usageErrorf("%q is not an accepted reference; use an explicit regular-file path, a full SHA256, an AIR URN with @<versionId>, or version:<id>", ref)
	}
	return Request{InputIndex: index, Kind: RequestPath, Ref: ref, Path: ref}, nil
}

// ParseRequests validates an ordered list of CLI references before any
// I/O and collapses exact duplicate ref strings idempotently, keeping the
// first appearance order. Distinct refs that later resolve to the same
// canonical identity stay separate results; the anchor logic treats them
// idempotently. An empty set is a usage error.
func ParseRequests(refs []string) ([]Request, error) {
	if len(refs) == 0 {
		return nil, usageErrorf("resolve requires at least one resource reference")
	}
	seen := make(map[string]bool, len(refs))
	requests := make([]Request, 0, len(refs))
	for _, ref := range refs {
		if seen[ref] {
			continue // exact duplicate: idempotent, keep the first appearance
		}
		seen[ref] = true
		req, err := ParseRef(len(requests), ref)
		if err != nil {
			return nil, err
		}
		requests = append(requests, req)
	}
	return requests, nil
}

// LooksLikePath reports whether ref has filesystem path syntax: a
// directory separator, a leading "." or "~", a Windows volume prefix
// ("C:"), or a final element containing a dot (an extension, e.g.
// model.safetensors). Bare words, integers, and short hashes are
// deliberately not path-shaped, so they fail as usage errors instead of
// being silently probed as local files (AC6). It is pure and OS-
// independent: Windows-style separators are recognized on every platform.
func LooksLikePath(ref string) bool {
	if ref == "" {
		return false
	}
	if strings.ContainsAny(ref, `/\`) {
		return true
	}
	if strings.HasPrefix(ref, ".") || strings.HasPrefix(ref, "~") {
		return true
	}
	if len(ref) >= 2 && ref[1] == ':' && isASCIIAlpha(ref[0]) {
		return true
	}
	return strings.Contains(ref, ".")
}

// Resolve evaluates one declared resource set against the reviewed
// policy. A nil error means the evaluation completed (Ready may be true
// or false); a non-nil error is a usage or operational failure and never
// carries a ready verdict. Lookups are sequential and deduplicated in
// memory for this invocation only; nothing is persisted.
func Resolve(ctx context.Context, requests []Request, policy Policy, env Environment) (Report, error) {
	if err := validateRequests(requests, env); err != nil {
		return Report{}, err
	}
	policy = normalizePolicy(policy)
	rep := Report{
		SchemaVersion: SchemaVersion,
		PolicyVersion: policy.Version,
		Resources:     make([]ResourceResult, 0, len(requests)),
		Compatibility: []CompatibilityResult{},
		Findings:      []Finding{},
	}

	cache := newLookupCache()
	for i := range requests {
		res, findings, err := resolveResource(ctx, i, requests[i], env, cache)
		if err != nil {
			return Report{}, err
		}
		for _, f := range findings {
			res.Issues = append(res.Issues, Issue{Code: f.Code, Message: f.Message})
		}
		rep.Resources = append(rep.Resources, res)
		rep.Findings = append(rep.Findings, findings...)
	}

	anchorIndex, anchorFindings := findAnchor(rep.Resources, policy)
	rep.Findings = append(rep.Findings, anchorFindings...)
	if anchorIndex >= 0 {
		compat, compatFindings := policy.evaluateCompatibility(rep.Resources, anchorIndex)
		rep.Compatibility = compat
		rep.Findings = append(rep.Findings, compatFindings...)
	}

	rep.Ready = readyFor(rep)
	rep.Summary = Summary{
		ResourceCount:        len(rep.Resources),
		BlockingFindingCount: countBlocking(rep.Findings),
	}
	return rep, nil
}

// validateRequests re-checks request structs so Resolve is safe for
// programmatic callers as well as the CLI parser. It performs no I/O.
func validateRequests(requests []Request, env Environment) error {
	if len(requests) == 0 {
		return usageErrorf("resolve requires at least one resource reference")
	}
	if env.Stat == nil || env.HashFile == nil || env.VersionByID == nil ||
		env.VersionByHash == nil || env.IsNotFound == nil {
		return fmt.Errorf("resolver: environment is incomplete")
	}
	for i, r := range requests {
		if strings.TrimSpace(r.Ref) == "" {
			return usageErrorf("resource %d has an empty reference", i)
		}
		switch r.Kind {
		case RequestPath:
			if strings.TrimSpace(r.Path) == "" {
				return usageErrorf("resource %d is a path reference without a path", i)
			}
			if r.SHA256 != "" || r.VersionID != 0 || r.AIR != "" {
				return usageErrorf("resource %d combines a path with another identity claim", i)
			}
			if r.ExpectedVersionID < 0 {
				return usageErrorf("resource %d has a negative expected version id", i)
			}
		case RequestSHA256:
			if len(r.SHA256) != 64 || !isHexString(r.SHA256) {
				return usageErrorf("resource %d does not carry a full 64-character SHA256", i)
			}
			if r.Path != "" || r.VersionID != 0 || r.ExpectedVersionID != 0 || r.AIR != "" {
				return usageErrorf("resource %d combines a hash with another identity claim", i)
			}
		case RequestAirVersion:
			if r.VersionID <= 0 || r.AIR == "" {
				return usageErrorf("resource %d is an AIR reference without an exact version", i)
			}
			if r.Path != "" || r.SHA256 != "" || r.ExpectedVersionID != 0 {
				return usageErrorf("resource %d combines AIR with another identity claim", i)
			}
		case RequestVersionID:
			if r.VersionID <= 0 {
				return usageErrorf("resource %d needs a positive version id", i)
			}
			if r.Path != "" || r.SHA256 != "" || r.AIR != "" || r.ExpectedVersionID != 0 {
				return usageErrorf("resource %d combines a version id with another identity claim", i)
			}
		default:
			return usageErrorf("resource %d has unknown kind %q", i, r.Kind)
		}
	}
	return nil
}

// normalizePolicy replaces only missing policy fields with resolver-v1
// defaults, so a zero Policy still evaluates deterministically.
func normalizePolicy(p Policy) Policy {
	def := DefaultPolicy()
	if p.Version == "" {
		p.Version = def.Version
	}
	if p.AnchorType == "" {
		p.AnchorType = def.AnchorType
	}
	if len(p.DependentTypes) == 0 {
		p.DependentTypes = def.DependentTypes
	}
	return p
}

// resolveResource dispatches one request to its evidence path.
func resolveResource(ctx context.Context, index int, req Request, env Environment, cache *lookupCache) (ResourceResult, []Finding, error) {
	res := ResourceResult{
		InputIndex:   index,
		RequestedRef: req.Ref,
		Kind:         req.Kind,
		Resolution:   ResolutionUnresolved,
		VisibleFiles: []FileMetadata{},
		Issues:       []Issue{},
	}
	switch req.Kind {
	case RequestPath:
		return resolvePath(ctx, req, env, cache, res)
	case RequestSHA256:
		return resolveSHA256(ctx, req, env, cache, res)
	case RequestAirVersion, RequestVersionID:
		return resolveVersionRef(ctx, req, env, cache, res)
	default:
		return res, nil, usageErrorf("unknown request kind %q", req.Kind)
	}
}

// resolvePath gathers local evidence (existence, content hash) then the
// registry evidence for that exact content. Missing paths complete with a
// missing_resource finding; directories are usage errors; read/hash
// failures are operational.
func resolvePath(ctx context.Context, req Request, env Environment, cache *lookupCache, res ResourceResult) (ResourceResult, []Finding, error) {
	status, err := env.Stat(req.Path)
	if err != nil {
		return res, nil, fmt.Errorf("resolver: stat %q: %w", req.Path, err)
	}
	if !status.Exists {
		res.Local.State = LocalMissing
		res.Local.Path = req.Path
		f := newFinding(CodeMissingResource, []int{res.InputIndex},
			fmt.Sprintf("explicit path %q does not exist", req.Path),
			Evidence{Path: req.Path, Reference: req.Ref})
		return res, []Finding{f}, nil
	}
	if !status.Regular {
		return res, nil, usageErrorf("%q is not a regular file; resolve reads explicit files only", req.Path)
	}
	hash, err := env.HashFile(req.Path)
	if err != nil {
		return res, nil, fmt.Errorf("resolver: hash %q: %w", req.Path, err)
	}
	hash = strings.ToUpper(strings.TrimSpace(hash))
	if len(hash) != 64 || !isHexString(hash) {
		return res, nil, fmt.Errorf("resolver: hash of %q is not a 64-character SHA256", req.Path)
	}
	res.Local.Path = req.Path
	res.Local.ComputedSHA256 = hash

	meta, found, err := cache.versionByHash(ctx, env, hash)
	if err != nil {
		return res, nil, fmt.Errorf("resolver: lookup hash %s: %w", hash, err)
	}
	if !found {
		res.Local.State = LocalUnresolved
		f := newFinding(CodeUnresolvedResource, []int{res.InputIndex},
			fmt.Sprintf("content %s is not registered on Civitai", hash),
			Evidence{Path: req.Path, Reference: req.Ref, Actual: &Identity{SHA256: hash}})
		return res, []Finding{f}, nil
	}

	fileFindings := applyVersion(&res, meta, hash, req.AIR)
	var findings []Finding
	wrong := req.ExpectedVersionID > 0 && meta.VersionID != req.ExpectedVersionID
	if wrong {
		expected, err := expectedIdentity(ctx, env, cache, req.ExpectedVersionID)
		if err != nil {
			return res, nil, fmt.Errorf("resolver: lookup expected version %d: %w", req.ExpectedVersionID, err)
		}
		actual := identityFromVersion(meta)
		actual.SHA256 = hash
		findings = append(findings, newFinding(CodeWrongVersion, []int{res.InputIndex},
			fmt.Sprintf("content %s resolves to version %d, but version %d was expected", hash, meta.VersionID, req.ExpectedVersionID),
			Evidence{Path: req.Path, Reference: req.Ref, Expected: &expected, Actual: &actual}))
	}
	findings = append(findings, fileFindings...)
	if wrong {
		res.Resolution = ResolutionMismatch
		res.Local.State = LocalMismatch
		return res, findings, nil
	}
	if res.Resolution == ResolutionResolved {
		res.Local.State = LocalPresent
	} else {
		res.Local.State = LocalUnresolved
	}
	return res, findings, nil
}

// resolveSHA256 resolves a full content hash to canonical registry
// identity. No path exists, so local presence stays unverified even when
// the hash binds exactly one visible file.
func resolveSHA256(ctx context.Context, req Request, env Environment, cache *lookupCache, res ResourceResult) (ResourceResult, []Finding, error) {
	res.Local.State = LocalUnverified
	hash := strings.ToUpper(strings.TrimSpace(req.SHA256))
	meta, found, err := cache.versionByHash(ctx, env, hash)
	if err != nil {
		return res, nil, fmt.Errorf("resolver: lookup hash %s: %w", hash, err)
	}
	var findings []Finding
	if !found {
		findings = append(findings, newFinding(CodeUnresolvedResource, []int{res.InputIndex},
			fmt.Sprintf("hash %s is not registered on Civitai", hash),
			Evidence{Reference: req.Ref, Actual: &Identity{SHA256: hash}}))
	} else {
		findings = append(findings, applyVersion(&res, meta, hash, req.AIR)...)
	}
	findings = append(findings, unverifiedFinding(res.InputIndex, req.Ref))
	return res, findings, nil
}

// resolveVersionRef resolves version:<id> and versioned AIR references by
// their exact numeric version id (the AIR version is authoritative). No
// content is read, so local presence stays unverified.
func resolveVersionRef(ctx context.Context, req Request, env Environment, cache *lookupCache, res ResourceResult) (ResourceResult, []Finding, error) {
	res.Local.State = LocalUnverified
	meta, found, err := cache.versionByID(ctx, env, req.VersionID)
	if err != nil {
		return res, nil, fmt.Errorf("resolver: lookup version %d: %w", req.VersionID, err)
	}
	var findings []Finding
	if !found {
		findings = append(findings, newFinding(CodeUnresolvedResource, []int{res.InputIndex},
			fmt.Sprintf("version %d is not registered on Civitai", req.VersionID),
			Evidence{Reference: req.Ref, Actual: &Identity{VersionID: req.VersionID, AIR: req.AIR}}))
		findings = append(findings, unverifiedFinding(res.InputIndex, req.Ref))
		return res, findings, nil
	}
	if meta.VersionID != req.VersionID {
		expected := Identity{VersionID: req.VersionID, AIR: req.AIR}
		actual := identityFromVersion(meta)
		f := newFinding(CodeWrongVersion, []int{res.InputIndex},
			fmt.Sprintf("registry returned version %d for requested version %d", meta.VersionID, req.VersionID),
			Evidence{Reference: req.Ref, Expected: &expected, Actual: &actual})
		findings = append(findings, f)
		findings = append(findings, applyVersion(&res, meta, "", req.AIR)...)
		res.Resolution = ResolutionMismatch
	} else {
		findings = append(findings, applyVersion(&res, meta, "", req.AIR)...)
	}
	findings = append(findings, unverifiedFinding(res.InputIndex, req.Ref))
	return res, findings, nil
}

// applyVersion projects registry metadata onto the result and, when a
// content hash is present, binds file identity by exact SHA256 against
// every visible file. It never falls back to the primary or first file.
func applyVersion(res *ResourceResult, meta VersionMetadata, contentSHA, fallbackAIR string) []Finding {
	res.Identity = identityFromVersion(meta)
	if res.Identity.AIR == "" {
		res.Identity.AIR = fallbackAIR
	}
	res.Names = Names{Model: meta.ModelName, Version: meta.Name}
	res.VisibleFiles = append([]FileMetadata{}, meta.Files...)

	if meta.ModelID <= 0 {
		res.Resolution = ResolutionUnresolved
		actual := res.Identity
		f := newFinding(CodeUnresolvedResource, []int{res.InputIndex},
			fmt.Sprintf("registry version %d carries no model id; canonical identity is incomplete", meta.VersionID),
			Evidence{Reference: res.RequestedRef, Expected: &Identity{VersionID: meta.VersionID, AIR: res.Identity.AIR}, Actual: &actual})
		return []Finding{f}
	}
	if contentSHA == "" {
		res.Resolution = ResolutionResolved
		return nil
	}

	matches := matchFiles(meta.Files, contentSHA)
	switch len(matches) {
	case 1:
		file := matches[0]
		res.Identity.FileID = file.FileID
		res.Identity.SHA256 = contentSHA
		res.Names.File = file.Name
		res.Local.MatchedFileID = file.FileID
		res.Local.ComputedSHA256 = contentSHA
		res.Resolution = ResolutionResolved
		return nil
	case 0:
		res.Resolution = ResolutionUnresolved
		actual := res.Identity
		actual.SHA256 = contentSHA
		if hasSHA256Evidence(meta.Files) {
			f := newFinding(CodeLocalHashMismatch, []int{res.InputIndex},
				fmt.Sprintf("content hash %s matches no visible file of version %d", contentSHA, meta.VersionID),
				Evidence{Reference: res.RequestedRef, Actual: &actual})
			return []Finding{f}
		}
		f := newFinding(CodeUnresolvedFileIdentity, []int{res.InputIndex},
			fmt.Sprintf("version %d carries no SHA256 file evidence to bind the content", meta.VersionID),
			Evidence{Reference: res.RequestedRef, Actual: &actual})
		return []Finding{f}
	default:
		res.Resolution = ResolutionAmbiguousFile
		ids := make([]int, len(matches))
		for i, file := range matches {
			ids[i] = file.FileID
		}
		actual := res.Identity
		actual.SHA256 = contentSHA
		f := newFinding(CodeAmbiguousFileIdentity, []int{res.InputIndex},
			fmt.Sprintf("content hash %s matches %d visible files of version %d; no file identity is guessed", contentSHA, len(matches), meta.VersionID),
			Evidence{Reference: res.RequestedRef, Actual: &actual})
		f.Message += fmt.Sprintf(" (file ids %v)", ids)
		return []Finding{f}
	}
}

// matchFiles returns every visible file whose projected SHA256 equals
// hash, case-insensitively (the registry emits uppercase, but the
// comparison is tolerant while identity remains exact).
func matchFiles(files []FileMetadata, hash string) []FileMetadata {
	var matches []FileMetadata
	for _, f := range files {
		if f.SHA256 != "" && strings.EqualFold(f.SHA256, hash) {
			matches = append(matches, f)
		}
	}
	return matches
}

// hasSHA256Evidence reports whether any visible file exposes a SHA256.
func hasSHA256Evidence(files []FileMetadata) bool {
	for _, f := range files {
		if f.SHA256 != "" {
			return true
		}
	}
	return false
}

// identityFromVersion projects canonical ids, AIR, and base metadata.
func identityFromVersion(meta VersionMetadata) Identity {
	return Identity{
		ModelID:       meta.ModelID,
		VersionID:     meta.VersionID,
		AIR:           meta.AIR,
		ModelType:     meta.ModelType,
		BaseModel:     meta.BaseModel,
		BaseModelType: meta.BaseModelType,
	}
}

// expectedIdentity fetches the explicitly expected version's metadata for
// wrong_version evidence. A registry 404 keeps the known id only; other
// failures stay operational.
func expectedIdentity(ctx context.Context, env Environment, cache *lookupCache, id int) (Identity, error) {
	meta, found, err := cache.versionByID(ctx, env, id)
	if err != nil {
		return Identity{}, err
	}
	if !found {
		return Identity{VersionID: id}, nil
	}
	return identityFromVersion(meta), nil
}

// unverifiedFinding builds the blocking local_presence_unverified finding
// shared by all remote-only reference forms.
func unverifiedFinding(index int, ref string) Finding {
	return newFinding(CodeLocalPresenceUnverified, []int{index},
		"no explicit local path was supplied; local presence cannot be verified",
		Evidence{Reference: ref})
}

// findAnchor locates the single distinct canonical Checkpoint resource.
// Zero yields missing_anchor; two or more distinct identities yield
// conflicting_anchor; repeated identical identities are idempotent and
// anchor on the first appearance. It returns -1 when no anchor exists.
func findAnchor(results []ResourceResult, policy Policy) (int, []Finding) {
	type anchorKey struct {
		modelID   int
		versionID int
	}
	indexes := make(map[anchorKey][]int)
	var order []anchorKey
	for i := range results {
		r := results[i]
		if r.Resolution != ResolutionResolved || !policy.isAnchorType(r.Identity.ModelType) {
			continue
		}
		if r.Identity.ModelID <= 0 || r.Identity.VersionID <= 0 {
			continue
		}
		k := anchorKey{r.Identity.ModelID, r.Identity.VersionID}
		if _, ok := indexes[k]; !ok {
			order = append(order, k)
		}
		indexes[k] = append(indexes[k], i)
	}

	if len(order) == 0 {
		f := newFinding(CodeMissingAnchor, nil,
			"the set has no resolved Checkpoint resource to act as the compatibility anchor", Evidence{})
		f.RuleID = RuleAnchorCheckpoint
		return -1, []Finding{f}
	}
	if len(order) > 1 {
		var all []int
		for _, k := range order {
			all = append(all, indexes[k]...)
		}
		sort.Ints(all)
		f := newFinding(CodeConflictingAnchor, all,
			"the set contains more than one distinct Checkpoint identity", Evidence{})
		f.RuleID = RuleAnchorCheckpoint
		return -1, []Finding{f}
	}
	return indexes[order[0]][0], nil
}

// readyFor reduces the report to the fail-closed verdict: every resource
// must have canonical identity and exact local presence, no blocking
// finding may exist, and every compatibility result must be compatible.
func readyFor(rep Report) bool {
	if len(rep.Resources) == 0 {
		return false
	}
	for _, r := range rep.Resources {
		if r.Resolution != ResolutionResolved || r.Local.State != LocalPresent {
			return false
		}
	}
	for _, f := range rep.Findings {
		if f.Blocking {
			return false
		}
	}
	for _, c := range rep.Compatibility {
		if c.Status != StatusCompatible {
			return false
		}
	}
	return true
}

// countBlocking counts the blocking findings in evaluation order.
func countBlocking(findings []Finding) int {
	n := 0
	for _, f := range findings {
		if f.Blocking {
			n++
		}
	}
	return n
}

// newFinding builds a blocking finding with a stable code and a copied
// resource-index slice (never nil, so JSON renders [] instead of null).
func newFinding(code string, indexes []int, message string, ev Evidence) Finding {
	idx := make([]int, len(indexes))
	copy(idx, indexes)
	return Finding{
		Code:            code,
		Blocking:        true,
		ResourceIndexes: idx,
		Message:         message,
		Evidence:        ev,
	}
}

// versionLookup is one memoized registry lookup: either metadata or a
// remembered 404 (so repeated refs never repeat a GET).
type versionLookup struct {
	meta     VersionMetadata
	notFound bool
}

// lookupCache deduplicates identical GETs inside one Resolve invocation.
// It is not a persistent cache and is never written anywhere.
type lookupCache struct {
	byID   map[int]versionLookup
	byHash map[string]versionLookup
}

// newLookupCache returns an empty invocation-scoped lookup cache.
func newLookupCache() *lookupCache {
	return &lookupCache{
		byID:   make(map[int]versionLookup),
		byHash: make(map[string]versionLookup),
	}
}

// versionByID returns memoized metadata for an exact version id. A
// registry 404 is a completed not-found (found=false, nil error); every
// other failure stays operational.
func (c *lookupCache) versionByID(ctx context.Context, env Environment, id int) (VersionMetadata, bool, error) {
	if l, ok := c.byID[id]; ok {
		return l.meta, !l.notFound, nil
	}
	meta, err := env.VersionByID(ctx, id)
	if err != nil {
		if env.IsNotFound(err) {
			c.byID[id] = versionLookup{notFound: true}
			return VersionMetadata{}, false, nil
		}
		return VersionMetadata{}, false, err
	}
	if meta.VersionID <= 0 {
		return VersionMetadata{}, false, fmt.Errorf("registry returned version data without a version id")
	}
	c.byID[id] = versionLookup{meta: meta}
	return meta, true, nil
}

// versionByHash returns memoized metadata for a full SHA256. The same
// 404-vs-operational split applies.
func (c *lookupCache) versionByHash(ctx context.Context, env Environment, hash string) (VersionMetadata, bool, error) {
	key := strings.ToUpper(hash)
	if l, ok := c.byHash[key]; ok {
		return l.meta, !l.notFound, nil
	}
	meta, err := env.VersionByHash(ctx, key)
	if err != nil {
		if env.IsNotFound(err) {
			c.byHash[key] = versionLookup{notFound: true}
			return VersionMetadata{}, false, nil
		}
		return VersionMetadata{}, false, err
	}
	if meta.VersionID <= 0 {
		return VersionMetadata{}, false, fmt.Errorf("registry returned version data without a version id")
	}
	c.byHash[key] = versionLookup{meta: meta}
	return meta, true, nil
}

// hasAIRPrefix reports whether ref starts with the AIR URN scheme,
// case-insensitively (URN schemes are case-insensitive).
func hasAIRPrefix(ref string) bool {
	return strings.HasPrefix(strings.ToLower(ref), "urn:air:")
}

// parseAIRVersion extracts the exact numeric version from an AIR URN
// shaped urn:air:<ecosystem>:<type>:<source>:<id>@<version>[+<fileId>][.<format>].
// Only the version is authoritative; the rest is display metadata.
func parseAIRVersion(ref string) (int, error) {
	colon := strings.LastIndex(ref, ":")
	if colon < 0 || colon == len(ref)-1 {
		return 0, usageErrorf("%q is not an AIR URN with an exact @<versionId>", ref)
	}
	tail := ref[colon+1:]
	at := strings.LastIndex(tail, "@")
	if at <= 0 {
		return 0, usageErrorf("%q must include an exact @<versionId>", ref)
	}
	versionPart := tail[at+1:]
	if cut := strings.IndexAny(versionPart, "+."); cut >= 0 {
		versionPart = versionPart[:cut]
	}
	id, err := parsePositiveID(versionPart)
	if err != nil {
		return 0, usageErrorf("%q carries no valid @<versionId>", ref)
	}
	return id, nil
}

// parsePositiveID parses a canonical positive decimal id: digits only, no
// sign, no leading zeros, and > 0.
func parsePositiveID(s string) (int, error) {
	if s == "" || !isAllDigits(s) {
		return 0, fmt.Errorf("not a positive decimal id")
	}
	id, err := strconv.Atoi(s)
	if err != nil || id <= 0 || strconv.Itoa(id) != s {
		return 0, fmt.Errorf("not a canonical positive decimal id")
	}
	return id, nil
}

// isHexString reports whether s is non-empty and contains only hex digits.
func isHexString(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

// isAllDigits reports whether s is non-empty and contains only 0-9.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isASCIIAlpha reports whether b is an ASCII letter (Windows volume
// prefix detection).
func isASCIIAlpha(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}
