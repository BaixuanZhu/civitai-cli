package compare

import (
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// ScalarDiff reports baseline-to-candidate scalar transitions. State is
// applied when every operand was supplied non-null; otherwise it is
// indeterminate and Changes is null.
type ScalarDiff struct {
	State   string         `json:"state"`
	Changes []ScalarChange `json:"changes"`
}

// ScalarChange is one confirmed scalar transition from the baseline.
type ScalarChange struct {
	Subject int `json:"subject"`
	From    any `json:"from"`
	To      any `json:"to"`
}

// SetDiff reports exact-string set additions/removals against the baseline.
// State is applied when every operand was supplied non-null; otherwise it
// is indeterminate and no add/remove arrays are fabricated.
type SetDiff struct {
	State   string      `json:"state"`
	Changes []SetChange `json:"changes"`
}

// SetChange is the directional set delta for one later subject. Added and
// Removed are lexicographically sorted and never nil when applied.
type SetChange struct {
	Subject int      `json:"subject"`
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
}

// NumberDiff reports exact integer deltas against the baseline. Only
// non-zero deltas are emitted.
type NumberDiff struct {
	State   string         `json:"state"`
	Changes []NumberChange `json:"changes"`
}

// NumberChange is one exact integer transition and its delta (to - from).
type NumberChange struct {
	Subject int   `json:"subject"`
	From    int64 `json:"from"`
	To      int64 `json:"to"`
	Delta   int64 `json:"delta"`
}

// FilesDiff reports baseline-to-candidate file pairing. State is applied
// only when the baseline files operand is supplied; per-candidate entries
// carry their own state. Match basis is explicit: fileId, sha256, or
// nameType, never array position or size.
type FilesDiff struct {
	State   string            `json:"state"`
	Changes []FileSubjectDiff `json:"changes"`
}

// FileSubjectDiff is the file delta for one later subject. Pairs, Added,
// and Removed are non-nil when State is applied.
type FileSubjectDiff struct {
	Subject int        `json:"subject"`
	State   string     `json:"state"`
	Pairs   []FilePair `json:"pairs"`
	Added   []FileRef  `json:"added"`
	Removed []FileRef  `json:"removed"`
}

// FilePair is a matched baseline/candidate file pair and its confirmed
// within-pair changes. Hashes keeps its own state so absent hash registries
// are not compared as empty.
type FilePair struct {
	MatchBasis      string       `json:"matchBasis"`
	BaselineFileID  int64        `json:"baselineFileId,omitempty"`
	CandidateFileID int64        `json:"candidateFileId,omitempty"`
	Changes         []FileChange `json:"changes"`
	Hashes          HashDiff     `json:"hashes"`
}

// FileChange is one confirmed matched-file fact transition.
type FileChange struct {
	Field string `json:"field"`
	From  any    `json:"from"`
	To    any    `json:"to"`
}

// HashDiff reports per-algorithm hash evidence for a matched file pair.
// Known hexadecimal algorithms compare case-insensitively; unknown
// algorithms stay exact strings.
type HashDiff struct {
	State   string       `json:"state"`
	Added   []HashEntry  `json:"added"`
	Removed []HashEntry  `json:"removed"`
	Changed []HashChange `json:"changed"`
}

// HashChange is a changed value for an algorithm present on both sides.
type HashChange struct {
	Algorithm string `json:"algorithm"`
	From      string `json:"from"`
	To        string `json:"to"`
}

// FileRef is an unmatched registry file. Reason is set when the evidence
// for a pairing basis was ambiguous; unmatched files remain visible rather
// than being guessed into a pair.
type FileRef struct {
	ID     int64  `json:"id,omitempty"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Reason string `json:"reason,omitempty"`
}

// RecipeDiff reports recipe-coverage count changes against the baseline.
// Only changed counts are emitted; State is applied only when every
// coverage operand was supplied.
type RecipeDiff struct {
	State   string              `json:"state"`
	Changes []RecipeCountChange `json:"changes"`
}

// RecipeCountChange is one coverage count transition and its delta.
type RecipeCountChange struct {
	Subject int    `json:"subject"`
	Field   string `json:"field"`
	From    int    `json:"from"`
	To      int    `json:"to"`
	Delta   int    `json:"delta"`
}

// deriveScalar reports from/to transitions between the baseline and each
// later subject; equal values produce no change record.
func deriveScalar(values []Value) any {
	if !allSupplied(values) {
		return &ScalarDiff{State: DerivedIndeterminate}
	}
	d := &ScalarDiff{State: DerivedApplied, Changes: []ScalarChange{}}
	for i := 1; i < len(values); i++ {
		if sameValue(values[0], values[i]) {
			continue
		}
		d.Changes = append(d.Changes, ScalarChange{
			Subject: i,
			From:    values[0].Value,
			To:      values[i].Value,
		})
	}
	return d
}

// deriveSet reports exact-string added/removed sets for every later
// subject. Both operands must be supplied non-null; input order and
// duplicate repetitions cannot create a difference.
func deriveSet(values []Value) any {
	if !allSupplied(values) {
		return &SetDiff{State: DerivedIndeterminate}
	}
	baseline, ok := stringSlice(values[0].Value)
	if !ok {
		return &SetDiff{State: DerivedIndeterminate}
	}
	d := &SetDiff{State: DerivedApplied, Changes: []SetChange{}}
	for i := 1; i < len(values); i++ {
		candidate, ok := stringSlice(values[i].Value)
		if !ok {
			return &SetDiff{State: DerivedIndeterminate}
		}
		added, removed := setDelta(baseline, candidate)
		d.Changes = append(d.Changes, SetChange{Subject: i, Added: added, Removed: removed})
	}
	return d
}

// deriveInt reports exact non-zero integer deltas for every later subject.
func deriveInt(values []Value) any {
	if !allSupplied(values) {
		return &NumberDiff{State: DerivedIndeterminate}
	}
	d := &NumberDiff{State: DerivedApplied, Changes: []NumberChange{}}
	from, ok := int64Value(values[0].Value)
	if !ok {
		return &NumberDiff{State: DerivedIndeterminate}
	}
	for i := 1; i < len(values); i++ {
		to, ok := int64Value(values[i].Value)
		if !ok {
			return &NumberDiff{State: DerivedIndeterminate}
		}
		if from == to {
			continue
		}
		d.Changes = append(d.Changes, NumberChange{Subject: i, From: from, To: to, Delta: to - from})
	}
	return d
}

// deriveFiles reports file additions, removals, and matched-file changes
// for every later subject. The baseline must be supplied for any pairing;
// a missing candidate stays an indeterminate per-subject entry.
func deriveFiles(values []Value) any {
	d := &FilesDiff{State: DerivedIndeterminate}
	if len(values) == 0 || values[0].State != StateValue {
		return d
	}
	baseline, ok := fileSlice(values[0].Value)
	if !ok {
		return d
	}
	if allSupplied(values) {
		d.State = DerivedApplied
	}
	changes := make([]FileSubjectDiff, 0, len(values)-1)
	for i := 1; i < len(values); i++ {
		if values[i].State != StateValue {
			changes = append(changes, FileSubjectDiff{Subject: i, State: DerivedIndeterminate})
			continue
		}
		candidate, ok := fileSlice(values[i].Value)
		if !ok {
			changes = append(changes, FileSubjectDiff{Subject: i, State: DerivedIndeterminate})
			continue
		}
		pairs, added, removed := pairFiles(baseline, candidate)
		changes = append(changes, FileSubjectDiff{
			Subject: i,
			State:   DerivedApplied,
			Pairs:   pairs,
			Added:   added,
			Removed: removed,
		})
	}
	d.Changes = changes
	return d
}

// deriveRecipe reports changed recipe-coverage counts against the baseline.
func deriveRecipe(values []Value) any {
	if !allSupplied(values) {
		return &RecipeDiff{State: DerivedIndeterminate}
	}
	baseline, ok := coverageValue(values[0].Value)
	if !ok {
		return &RecipeDiff{State: DerivedIndeterminate}
	}
	d := &RecipeDiff{State: DerivedApplied, Changes: []RecipeCountChange{}}
	for i := 1; i < len(values); i++ {
		candidate, ok := coverageValue(values[i].Value)
		if !ok {
			return &RecipeDiff{State: DerivedIndeterminate}
		}
		baseCounts := coverageCounts(baseline)
		candCounts := coverageCounts(candidate)
		for k := range baseCounts {
			if baseCounts[k].n == candCounts[k].n {
				continue
			}
			d.Changes = append(d.Changes, RecipeCountChange{
				Subject: i,
				Field:   baseCounts[k].field,
				From:    baseCounts[k].n,
				To:      candCounts[k].n,
				Delta:   candCounts[k].n - baseCounts[k].n,
			})
		}
	}
	return d
}

// allSupplied reports whether every value is a supplied non-null value.
func allSupplied(values []Value) bool {
	if len(values) == 0 {
		return false
	}
	for _, v := range values {
		if v.State != StateValue {
			return false
		}
	}
	return true
}

// stringSlice types a projected set value.
func stringSlice(v any) ([]string, bool) {
	s, ok := v.([]string)
	if !ok {
		return nil, false
	}
	return s, true
}

// int64Value types a projected integer value.
func int64Value(v any) (int64, bool) {
	i, ok := v.(int64)
	return i, ok
}

// fileSlice types a projected file-list value.
func fileSlice(v any) ([]FileValue, bool) {
	f, ok := v.([]FileValue)
	return f, ok
}

// coverageValue types a projected recipe-coverage value.
func coverageValue(v any) (RecipeCoverage, bool) {
	c, ok := v.(RecipeCoverage)
	return c, ok
}

// setDelta returns sorted candidate-only and baseline-only strings. Empty
// results are non-nil so applied operations with no changes render as [].
func setDelta(baseline, candidate []string) (added, removed []string) {
	baseSet := make(map[string]struct{}, len(baseline))
	for _, s := range baseline {
		baseSet[s] = struct{}{}
	}
	candSet := make(map[string]struct{}, len(candidate))
	for _, s := range candidate {
		candSet[s] = struct{}{}
	}
	added = []string{}
	removed = []string{}
	for _, s := range candidate {
		if _, ok := baseSet[s]; !ok {
			added = append(added, s)
		}
	}
	for _, s := range baseline {
		if _, ok := candSet[s]; !ok {
			removed = append(removed, s)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

// coverageCount is one named recipe-coverage count in registry order.
type coverageCount struct {
	field string
	n     int
}

// coverageCounts flattens a coverage object into the fixed comparison
// order: totals first, then the nine non-empty metadata counts.
func coverageCounts(cov RecipeCoverage) []coverageCount {
	return []coverageCount{
		{"imagesTotal", cov.ImagesTotal},
		{"hasMetaTrue", cov.HasMetaTrue},
		{"hasMetaFalse", cov.HasMetaFalse},
		{"hasMetaNull", cov.HasMetaNull},
		{"hasMetaMissing", cov.HasMetaMissing},
		{"metaPresent", cov.MetaPresent},
		{"declaredMetaMissing", cov.DeclaredMetaMissing},
		{"prompt", cov.NonEmpty.Prompt},
		{"negativePrompt", cov.NonEmpty.NegativePrompt},
		{"steps", cov.NonEmpty.Steps},
		{"cfgScale", cov.NonEmpty.CFGScale},
		{"sampler", cov.NonEmpty.Sampler},
		{"scheduler", cov.NonEmpty.Scheduler},
		{"seed", cov.NonEmpty.Seed},
		{"width", cov.NonEmpty.Width},
		{"height", cov.NonEmpty.Height},
	}
}

// canonicalFiles returns a stable copy ordered by registry ID, then exact
// name, then type. Files without an ID sort after files with one; the
// original order breaks full ties. API ordering alone cannot change a
// comparison result.
func canonicalFiles(files []FileValue) []FileValue {
	out := append([]FileValue{}, files...)
	sort.SliceStable(out, func(i, j int) bool {
		ai, aok := intOpt(out[i].ID)
		bi, bok := intOpt(out[j].ID)
		if aok != bok {
			return aok
		}
		if aok && ai != bi {
			return ai < bi
		}
		an, _ := stringOpt(out[i].Name)
		bn, _ := stringOpt(out[j].Name)
		if an != bn {
			return an < bn
		}
		at, _ := stringOpt(out[i].Type)
		bt, _ := stringOpt(out[j].Type)
		return at < bt
	})
	return out
}

// fileBasis is one step of the approved evidence cascade.
type fileBasis struct {
	name string
	key  func(FileValue) (string, bool)
}

// fileBases is the deterministic pairing cascade: unique file ID, then
// unique SHA256, then unique exact name/type.
var fileBases = []fileBasis{
	{name: "fileId", key: fileIDKey},
	{name: "sha256", key: fileSHA256Key},
	{name: "nameType", key: fileNameTypeKey},
}

// fileIDKey is the positive registry file ID.
func fileIDKey(f FileValue) (string, bool) {
	id, ok := intOpt(f.ID)
	if !ok || id <= 0 {
		return "", false
	}
	return strconv.FormatInt(id, 10), true
}

// fileSHA256Key is a supplied non-empty SHA256 normalized for matching.
func fileSHA256Key(f FileValue) (string, bool) {
	for _, h := range f.Hashes {
		if h.Algorithm == "SHA256" && h.Value != "" {
			return strings.ToUpper(h.Value), true
		}
	}
	return "", false
}

// fileNameTypeKey is the exact supplied (name, type) pair.
func fileNameTypeKey(f FileValue) (string, bool) {
	name, nok := stringOpt(f.Name)
	typ, tok := stringOpt(f.Type)
	if !nok || !tok {
		return "", false
	}
	return name + "\x00" + typ, true
}

// pairFiles matches baseline and candidate files through the evidence
// cascade. Only keys unique among the remaining files on both sides pair;
// ambiguous or residual files stay added/removed. Pair order follows the
// cascade then each side's canonical order.
func pairFiles(baseline, candidate []FileValue) (pairs []FilePair, added, removed []FileRef) {
	baseline = canonicalFiles(baseline)
	candidate = canonicalFiles(candidate)
	usedBase := make([]bool, len(baseline))
	usedCand := make([]bool, len(candidate))
	pairs = []FilePair{}
	for _, basis := range fileBases {
		baseIndex := indexUnused(baseline, usedBase, basis.key)
		candIndex := indexUnused(candidate, usedCand, basis.key)
		for i := range baseline {
			if usedBase[i] {
				continue
			}
			key, ok := basis.key(baseline[i])
			if !ok || len(baseIndex[key]) != 1 || len(candIndex[key]) != 1 {
				continue
			}
			j := candIndex[key][0]
			if usedCand[j] {
				continue
			}
			usedBase[i] = true
			usedCand[j] = true
			pairs = append(pairs, pairFile(baseline[i], candidate[j], basis.name))
		}
	}
	added = []FileRef{}
	for j := range candidate {
		if usedCand[j] {
			continue
		}
		added = append(added, fileRef(candidate[j], ambiguityReason(candidate[j], baseline, candidate)))
	}
	removed = []FileRef{}
	for i := range baseline {
		if usedBase[i] {
			continue
		}
		removed = append(removed, fileRef(baseline[i], ambiguityReason(baseline[i], baseline, candidate)))
	}
	return pairs, added, removed
}

// indexUnused maps each pairing key to the unused files carrying it.
func indexUnused(files []FileValue, used []bool, key func(FileValue) (string, bool)) map[string][]int {
	index := make(map[string][]int, len(files))
	for i := range files {
		if used[i] {
			continue
		}
		k, ok := key(files[i])
		if !ok {
			continue
		}
		index[k] = append(index[k], i)
	}
	return index
}

// ambiguityReason names the first cascade basis whose key is duplicated on
// either side, so an unmatched file's presence is explainable.
func ambiguityReason(f FileValue, baseline, candidate []FileValue) string {
	for _, basis := range fileBases {
		key, ok := basis.key(f)
		if !ok {
			continue
		}
		if countKey(baseline, basis.key, key) > 1 || countKey(candidate, basis.key, key) > 1 {
			return "ambiguous " + basis.name + " match"
		}
	}
	return ""
}

// countKey counts files carrying key in the given list.
func countKey(files []FileValue, key func(FileValue) (string, bool), want string) int {
	n := 0
	for _, f := range files {
		k, ok := key(f)
		if ok && k == want {
			n++
		}
	}
	return n
}

// pairFile compares one matched pair. Only fields supplied on both sides
// can produce a confirmed change; missing metadata stays indeterminate
// instead of being treated as empty.
func pairFile(baseline, candidate FileValue, basis string) FilePair {
	p := FilePair{
		MatchBasis: basis,
		Changes:    []FileChange{},
		Hashes:     hashDiff(baseline, candidate),
	}
	if id, ok := intOpt(baseline.ID); ok {
		p.BaselineFileID = id
	}
	if id, ok := intOpt(candidate.ID); ok {
		p.CandidateFileID = id
	}
	fields := []struct {
		name string
		a, b Optional
	}{
		{"name", baseline.Name, candidate.Name},
		{"type", baseline.Type, candidate.Type},
		{"primary", baseline.Primary, candidate.Primary},
		{"sizeKB", baseline.SizeKB, candidate.SizeKB},
		{"format", baseline.Format, candidate.Format},
		{"fp", baseline.FP, candidate.FP},
		{"size", baseline.Size, candidate.Size},
		{"pickleScanResult", baseline.PickleScanResult, candidate.PickleScanResult},
		{"virusScanResult", baseline.VirusScanResult, candidate.VirusScanResult},
		{"scannedAt", baseline.ScannedAt, candidate.ScannedAt},
	}
	for _, f := range fields {
		if f.a.State != StateValue || f.b.State != StateValue {
			continue
		}
		if reflect.DeepEqual(f.a.Value, f.b.Value) {
			continue
		}
		p.Changes = append(p.Changes, FileChange{Field: f.name, From: f.a.Value, To: f.b.Value})
	}
	return p
}

// hashDiff compares hash registries by exact algorithm name. A timestamp-
// only rescan produces no hash change; known hex algorithms ignore case
// for equality while original values stay in the aligned facts.
func hashDiff(baseline, candidate FileValue) HashDiff {
	if baseline.HashesState != StateValue || candidate.HashesState != StateValue {
		return HashDiff{State: DerivedIndeterminate}
	}
	baseMap := hashMap(baseline.Hashes)
	candMap := hashMap(candidate.Hashes)
	d := HashDiff{
		State:   DerivedApplied,
		Added:   []HashEntry{},
		Removed: []HashEntry{},
		Changed: []HashChange{},
	}
	for algo, baseVal := range baseMap {
		candVal, ok := candMap[algo]
		if !ok {
			d.Removed = append(d.Removed, HashEntry{Algorithm: algo, Value: baseVal})
			continue
		}
		if !hashValueEqual(algo, baseVal, candVal) {
			d.Changed = append(d.Changed, HashChange{Algorithm: algo, From: baseVal, To: candVal})
		}
	}
	for algo, candVal := range candMap {
		if _, ok := baseMap[algo]; !ok {
			d.Added = append(d.Added, HashEntry{Algorithm: algo, Value: candVal})
		}
	}
	sort.Slice(d.Added, func(i, j int) bool { return d.Added[i].Algorithm < d.Added[j].Algorithm })
	sort.Slice(d.Removed, func(i, j int) bool { return d.Removed[i].Algorithm < d.Removed[j].Algorithm })
	sort.Slice(d.Changed, func(i, j int) bool { return d.Changed[i].Algorithm < d.Changed[j].Algorithm })
	return d
}

// hashMap indexes hash entries by algorithm.
func hashMap(entries []HashEntry) map[string]string {
	m := make(map[string]string, len(entries))
	for _, h := range entries {
		m[h.Algorithm] = h.Value
	}
	return m
}

// isKnownHexHashAlgorithm reports whether the algorithm's value is a
// hexadecimal digest: case-only differences are the same value. Unknown
// algorithms remain exact strings.
func isKnownHexHashAlgorithm(algorithm string) bool {
	switch algorithm {
	case "AutoV1", "AutoV2", "AutoV3", "SHA256", "SHA256_12", "BLAKE3", "CRC32":
		return true
	}
	return false
}

// hashValueEqual compares hash values for one algorithm.
func hashValueEqual(algorithm, a, b string) bool {
	if isKnownHexHashAlgorithm(algorithm) && isHexString(a) && isHexString(b) {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// isHexString reports whether s is a non-empty hexadecimal string.
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

// intOpt extracts a supplied integer attribute.
func intOpt(o Optional) (int64, bool) {
	if o.State != StateValue {
		return 0, false
	}
	i, ok := o.Value.(int64)
	return i, ok
}

// stringOpt extracts a supplied string attribute.
func stringOpt(o Optional) (string, bool) {
	if o.State != StateValue {
		return "", false
	}
	s, ok := o.Value.(string)
	return s, ok
}

// fileRef reduces one unmatched file to its visible identity.
func fileRef(f FileValue, reason string) FileRef {
	r := FileRef{Reason: reason}
	if id, ok := intOpt(f.ID); ok {
		r.ID = id
	}
	if name, ok := stringOpt(f.Name); ok {
		r.Name = name
	}
	if typ, ok := stringOpt(f.Type); ok {
		r.Type = typ
	}
	return r
}
