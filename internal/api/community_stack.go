package api

import (
	"encoding/json"
	"sort"
	"strings"
)

// Community stack summary constants (task 11-27-community-stack-summary).
// The view reuses the civ.recipe/v1 schema family with a distinct kind
// because it is a new projection of the same bounded response, not a change
// to an existing kind. The top limit is fixed at five so output stays
// compact and deterministic; a user-configurable bound is out of scope.
const (
	communityStackSummaryKind = "communityStackSummary"
	communityStackTopLimit    = 5

	communityStackBasisModelVersionID = "modelVersionId"
	communityStackBasisHash           = "hash"
	communityStackBasisName           = "name"

	communityStackTypeCheckpoint = "checkpoint"
	communityStackTypeLoRA       = "lora"
)

// communityStackCaveats returns the fixed caveat vocabulary of the stack
// summary. It is a fresh slice per call so no mutable package state exists:
// the sample is bounded and non-random, continuation is never followed, and
// the aggregates are descriptive evidence rather than recommendations.
func communityStackCaveats() []string {
	return []string{"boundedNonRandomSample", "continuationNotFollowed", "descriptiveEvidenceOnly"}
}

// CommunityResourceIdentity is the conservative identity one resource
// identity row is keyed by. Basis records which evidence selected the
// identity ("modelVersionId", "hash", or "name") and exactly that basis
// field is non-null; an ID-based identity is never merged with a
// hash/name-only one because the response does not prove their equivalence.
// Type is the canonical "checkpoint"/"lora", the exact other type string,
// or null for an untyped entry.
type CommunityResourceIdentity struct {
	Type           *string `json:"type"`
	Basis          string  `json:"basis"`
	ModelVersionID *int64  `json:"modelVersionId"`
	Hash           *string `json:"hash"`
	Name           *string `json:"name"`
}

// CommunityResourceUsage is one checkpoint or other-resource identity row:
// the number of distinct sampled images (by upstream response position)
// associated with the exact identity, never a raw resource-entry count.
type CommunityResourceUsage struct {
	Identity   CommunityResourceIdentity `json:"identity"`
	ImageCount int                       `json:"imageCount"`
}

// CommunityWeightSummary reports valid LoRA weight evidence for one
// identity. Median is computed only from finite normalized weights
// (duplicate equal weights within one image collapse, different weights in
// one image stay separate samples); it is null when no valid sample exists.
type CommunityWeightSummary struct {
	Median           *float64 `json:"median"`
	ImagesWithWeight int      `json:"imagesWithWeight"`
	SampleCount      int      `json:"sampleCount"`
}

// CommunityLoRAUsage is one LoRA identity row: distinct-image usage plus
// the weight evidence for that identity. Weight coverage is relative to
// this row's ImageCount, not to the whole sample.
type CommunityLoRAUsage struct {
	Identity   CommunityResourceIdentity `json:"identity"`
	ImageCount int                       `json:"imageCount"`
	Weight     CommunityWeightSummary    `json:"weight"`
}

// CommunityResourceGroup is the bounded top-identity view of one resource
// group. EntryCount includes unidentified entries assigned to the group;
// IdentifiedImageCount is a set of distinct response positions, not a sum
// of row counts. OtherIdentityCount is the number of unique identities
// omitted from Top; their image counts are deliberately not summed because
// image sets can overlap.
type CommunityResourceGroup struct {
	EntryCount           int                      `json:"entryCount"`
	IdentifiedImageCount int                      `json:"identifiedImageCount"`
	Top                  []CommunityResourceUsage `json:"top"`
	OtherIdentityCount   int                      `json:"otherIdentityCount"`
}

// CommunityLoRAGroup is the LoRA-specific group view; only LoRA rows carry
// weight summaries. Unknown or untyped resources are never assumed to be
// LoRAs and therefore never receive a median.
type CommunityLoRAGroup struct {
	EntryCount           int                  `json:"entryCount"`
	IdentifiedImageCount int                  `json:"identifiedImageCount"`
	Top                  []CommunityLoRAUsage `json:"top"`
	OtherIdentityCount   int                  `json:"otherIdentityCount"`
}

// CommunityResourceSummary aggregates the projected resource entries of
// the whole sample. EntryCount covers every decodable object entry from
// both resource arrays, including duplicates and unidentified entries;
// UnidentifiedEntryCount is the exact remainder. ImagesWithEntries counts
// distinct response positions with at least one projected entry.
type CommunityResourceSummary struct {
	EntryCount             int                    `json:"entryCount"`
	IdentifiedEntryCount   int                    `json:"identifiedEntryCount"`
	UnidentifiedEntryCount int                    `json:"unidentifiedEntryCount"`
	ImagesWithEntries      int                    `json:"imagesWithEntries"`
	Checkpoints            CommunityResourceGroup `json:"checkpoints"`
	LoRAs                  CommunityLoRAGroup     `json:"loras"`
	OtherResources         CommunityResourceGroup `json:"otherResources"`
}

// CommunityValueCount is one parameter value and the number of distinct
// sampled images observed with it. Value is an int64 for steps, a float64
// for cfgScale, and the exact string for sampler/scheduler.
type CommunityValueCount struct {
	Value      any `json:"value"`
	ImageCount int `json:"imageCount"`
}

// CommunityOtherValues aggregates every observed value outside the top
// rows: ImageCount is the observed-image count outside Top (so Top plus
// Other reconciles exactly to ObservedImages) and DistinctValueCount is
// the number of unique omitted values.
type CommunityOtherValues struct {
	ImageCount         int `json:"imageCount"`
	DistinctValueCount int `json:"distinctValueCount"`
}

// CommunityValueDistribution is one parameter's bounded distribution.
// ObservedImages counts only valid normalized values; SampledImages is the
// full response denominator (coverage.returned), so sparse or invalid
// items honestly reduce observed coverage.
type CommunityValueDistribution struct {
	ObservedImages int                   `json:"observedImages"`
	SampledImages  int                   `json:"sampledImages"`
	Top            []CommunityValueCount `json:"top"`
	Other          CommunityOtherValues  `json:"other"`
}

// CommunityParameterSummary carries the four approved parameter
// distributions. All four are already first-class normalized fields in the
// community projection, so no new fetch or interpretation is needed.
type CommunityParameterSummary struct {
	Steps     CommunityValueDistribution `json:"steps"`
	CFGScale  CommunityValueDistribution `json:"cfgScale"`
	Sampler   CommunityValueDistribution `json:"sampler"`
	Scheduler CommunityValueDistribution `json:"scheduler"`
}

// CommunityTextStatusCoverage is the aggregate-only availability evidence
// for one text field. The four counts always sum to coverage.returned;
// ProvidedRate is provided/returned and null when nothing was returned.
// No text value is ever retained in this type.
type CommunityTextStatusCoverage struct {
	Provided             int      `json:"provided"`
	Empty                int      `json:"empty"`
	NotProvided          int      `json:"notProvided"`
	UnsupportedJSONShape int      `json:"unsupportedJsonShape"`
	ProvidedRate         *float64 `json:"providedRate"`
}

// add records one classified status. Statuses come only from
// recipeTextValue/communityMetadataStatus, so the default branch is the
// notProvided remainder rather than a silent drop.
func (c *CommunityTextStatusCoverage) add(status string) {
	switch status {
	case recipeStatusProvided:
		c.Provided++
	case recipeStatusEmpty:
		c.Empty++
	case recipeStatusUnsupported:
		c.UnsupportedJSONShape++
	default:
		c.NotProvided++
	}
}

// finishRate computes provided/returned once the counts are final; a zero
// denominator leaves the rate null instead of inventing 0 or 1.
func (c *CommunityTextStatusCoverage) finishRate(returned int) {
	if returned == 0 {
		return
	}
	rate := float64(c.Provided) / float64(returned)
	c.ProvidedRate = &rate
}

// CommunityTextAvailability aggregates prompt and negative-prompt
// availability without any text content. The member names keep the
// existing summary vocabulary so exact-key redaction tests can keep
// forbidding the prompt and negativePrompt keys themselves.
type CommunityTextAvailability struct {
	PromptStatus         CommunityTextStatusCoverage `json:"promptStatus"`
	NegativePromptStatus CommunityTextStatusCoverage `json:"negativePromptStatus"`
}

// CommunityStackSummary is the aggregate-only projection of one bounded
// community /images response. It structurally excludes observations,
// per-image identity, prompt/negative text, raw metadata, workflows,
// media/page locations, continuation tokens, and next-page URLs; the only
// continuation fact is the existing boolean coverage.ContinuationOffered.
type CommunityStackSummary struct {
	SchemaVersion    string                    `json:"schemaVersion"`
	Kind             string                    `json:"kind"`
	Provenance       RecipeProvenance          `json:"provenance"`
	Query            CommunityQuery            `json:"query"`
	Coverage         CommunityCoverage         `json:"coverage"`
	Caveats          []string                  `json:"caveats"`
	TopLimit         int                       `json:"topLimit"`
	Resources        CommunityResourceSummary  `json:"resources"`
	Parameters       CommunityParameterSummary `json:"parameters"`
	TextAvailability CommunityTextAvailability `json:"textAvailability"`
}

// ParseCommunityStackSummary decodes one bounded GET /images list body and
// projects the aggregate-only stack summary. versionID, limit, and sort
// carry the exact ParseCommunitySample contract and validation; the same
// single response is parsed once for normalized observations and once
// locally for status-only text availability, so the view never performs a
// second request, pagination, or alternate-sort retry.
func ParseCommunityStackSummary(raw []byte, versionID, limit int, sort string) (*CommunityStackSummary, error) {
	sample, err := ParseCommunitySample(raw, versionID, limit, sort)
	if err != nil {
		return nil, err
	}
	env, err := decodeImagesEnvelope(raw)
	if err != nil {
		return nil, err
	}
	items, err := decodeImagesItems(env)
	if err != nil {
		return nil, err
	}

	acc := newCommunityStackAccumulator()
	for _, obs := range sample.Observations {
		acc.addObservation(obs)
	}

	return &CommunityStackSummary{
		SchemaVersion: recipeSchemaVersion,
		Kind:          communityStackSummaryKind,
		Provenance:    sample.Provenance,
		Query:         sample.Query,
		Coverage:      sample.Coverage,
		Caveats:       communityStackCaveats(),
		TopLimit:      communityStackTopLimit,
		Resources:     acc.resourceSummary(),
		Parameters: CommunityParameterSummary{
			Steps:     communityStackDistribution(sample.Observations, "steps", stackValueSteps, sample.Coverage.Returned),
			CFGScale:  communityStackDistribution(sample.Observations, "cfgScale", stackValueCFGScale, sample.Coverage.Returned),
			Sampler:   communityStackDistribution(sample.Observations, "sampler", stackValueSampler, sample.Coverage.Returned),
			Scheduler: communityStackDistribution(sample.Observations, "scheduler", stackValueScheduler, sample.Coverage.Returned),
		},
		TextAvailability: communityStackTextAvailability(items, sample.Coverage.Returned),
	}, nil
}

// stackGroupKind is the internal resource grouping: the two reviewed types
// get named groups, everything else (including untyped entries) lands in
// the other group so unknown types are never aliased to checkpoint or LoRA.
type stackGroupKind int

const (
	stackGroupCheckpoints stackGroupKind = iota
	stackGroupLoRAs
	stackGroupOther
)

// stackIdentityKey is the canonical dedup key: type, basis, and the typed
// value. Keeping the basis inside the key is what prevents an ID-based
// identity from merging with a hash/name-only one, and keeping the type
// inside prevents the same ID used under different types from merging.
type stackIdentityKey struct {
	typeKey string // canonical "checkpoint"/"lora", exact other type, or "" for untyped
	basis   string
	id      int64  // only for basis modelVersionId
	str     string // exact hash or name for those bases
}

// identity projects the key to its public DTO: only the selected basis
// field is non-null, and the type is null for untyped entries.
func (k stackIdentityKey) identity() CommunityResourceIdentity {
	identity := CommunityResourceIdentity{Basis: k.basis}
	if k.typeKey != "" {
		t := k.typeKey
		identity.Type = &t
	}
	switch k.basis {
	case communityStackBasisModelVersionID:
		id := k.id
		identity.ModelVersionID = &id
	case communityStackBasisHash:
		h := k.str
		identity.Hash = &h
	case communityStackBasisName:
		n := k.str
		identity.Name = &n
	}
	return identity
}

// classifyCommunityStackType recognizes exactly the reviewed checkpoint and
// lora tokens (trimmed and case-folded for recognition only, emitted in
// canonical form). Any other non-empty type keeps its exact original string
// in the other group; absent, null, or whitespace-only types are untyped.
func classifyCommunityStackType(t *string) (stackGroupKind, string) {
	if t == nil || strings.TrimSpace(*t) == "" {
		return stackGroupOther, ""
	}
	switch strings.ToLower(strings.TrimSpace(*t)) {
	case communityStackTypeCheckpoint:
		return stackGroupCheckpoints, communityStackTypeCheckpoint
	case communityStackTypeLoRA:
		return stackGroupLoRAs, communityStackTypeLoRA
	default:
		return stackGroupOther, *t
	}
}

// communityStackIdentity applies the conservative precedence: a positive
// modelVersionId, then a non-empty hash, then a non-empty name. Trimming is
// only an emptiness test; the retained key string is exact and
// case-sensitive. A non-positive ID never suppresses a hash/name fallback.
// Top-level modelVersionIds, the requested version, origin, and weight
// never create identity.
func communityStackIdentity(r ObservedResource, typeKey string) (stackIdentityKey, bool) {
	if r.ModelVersionID != nil && *r.ModelVersionID > 0 {
		return stackIdentityKey{typeKey: typeKey, basis: communityStackBasisModelVersionID, id: *r.ModelVersionID}, true
	}
	if r.Hash != nil && strings.TrimSpace(*r.Hash) != "" {
		return stackIdentityKey{typeKey: typeKey, basis: communityStackBasisHash, str: *r.Hash}, true
	}
	if r.Name != nil && strings.TrimSpace(*r.Name) != "" {
		return stackIdentityKey{typeKey: typeKey, basis: communityStackBasisName, str: *r.Name}, true
	}
	return stackIdentityKey{}, false
}

// canonicalizeCommunityStackFloat maps signed zero to numeric zero so
// grouping, medians, and JSON output cannot diverge on a -0 bit pattern.
func canonicalizeCommunityStackFloat(v float64) float64 {
	if v == 0 {
		return 0
	}
	return v
}

// stackGroupAccumulator holds the per-image sets of one resource group.
// Sets are keyed by upstream response position, so duplicate resource
// entries inside one sampled image never inflate distinct-image counts.
type stackGroupAccumulator struct {
	entryCount       int
	identifiedImages map[int]bool
	identities       map[stackIdentityKey]map[int]bool
}

// communityStackAccumulator aggregates the whole sample before any sorting
// happens; all output ordering is decided once at final projection.
type communityStackAccumulator struct {
	entryCount           int
	identifiedEntryCount int
	imagesWithEntries    map[int]bool
	groups               map[stackGroupKind]*stackGroupAccumulator
	loraWeights          map[stackIdentityKey]map[int]map[float64]bool
}

// newCommunityStackAccumulator initializes every group up front so empty
// samples still produce zero-valued, non-nil group blocks.
func newCommunityStackAccumulator() *communityStackAccumulator {
	return &communityStackAccumulator{
		imagesWithEntries: map[int]bool{},
		groups: map[stackGroupKind]*stackGroupAccumulator{
			stackGroupCheckpoints: {identifiedImages: map[int]bool{}, identities: map[stackIdentityKey]map[int]bool{}},
			stackGroupLoRAs:       {identifiedImages: map[int]bool{}, identities: map[stackIdentityKey]map[int]bool{}},
			stackGroupOther:       {identifiedImages: map[int]bool{}, identities: map[stackIdentityKey]map[int]bool{}},
		},
		loraWeights: map[stackIdentityKey]map[int]map[float64]bool{},
	}
}

// addObservation folds one sampled image into the accumulator. The image
// unit is the upstream response position, never the nullable image ID, so
// repeated upstream IDs stay separate samples and missing IDs still count.
func (acc *communityStackAccumulator) addObservation(obs CommunityObservation) {
	position := obs.UpstreamPosition
	if len(obs.Resources) > 0 {
		acc.imagesWithEntries[position] = true
	}
	for _, resource := range obs.Resources {
		acc.entryCount++
		groupKind, typeKey := classifyCommunityStackType(resource.Type)
		group := acc.groups[groupKind]
		group.entryCount++
		key, ok := communityStackIdentity(resource, typeKey)
		if !ok {
			continue
		}
		acc.identifiedEntryCount++
		group.identifiedImages[position] = true
		positions := group.identities[key]
		if positions == nil {
			positions = map[int]bool{}
			group.identities[key] = positions
		}
		positions[position] = true
		if groupKind == stackGroupLoRAs && resource.Weight != nil {
			acc.addLoRAWeight(key, position, canonicalizeCommunityStackFloat(*resource.Weight))
		}
	}
}

// addLoRAWeight retains one valid normalized weight for one image/identity
// pair. Equal duplicates within the same image collapse into one sample;
// different weights in the same image remain separate samples.
func (acc *communityStackAccumulator) addLoRAWeight(key stackIdentityKey, position int, weight float64) {
	perImage := acc.loraWeights[key]
	if perImage == nil {
		perImage = map[int]map[float64]bool{}
		acc.loraWeights[key] = perImage
	}
	weights := perImage[position]
	if weights == nil {
		weights = map[float64]bool{}
		perImage[position] = weights
	}
	weights[weight] = true
}

// stackUsageRow is one identity's final distinct-image count before
// projection.
type stackUsageRow struct {
	key        stackIdentityKey
	imageCount int
}

// rows materializes the group's identity set in the deterministic total
// order: image count descending, type ascending (null sorts first
// bytewise), basis precedence, then numeric ID or exact string ascending.
// The key is unique, so the order is total and map iteration cannot leak.
func (g *stackGroupAccumulator) rows() []stackUsageRow {
	rows := make([]stackUsageRow, 0, len(g.identities))
	for key, positions := range g.identities {
		rows = append(rows, stackUsageRow{key: key, imageCount: len(positions)})
	}
	sort.Slice(rows, func(i, j int) bool {
		return compareStackUsageRows(rows[i], rows[j]) < 0
	})
	return rows
}

// compareStackUsageRows orders two usage rows; see rows for the contract.
func compareStackUsageRows(a, b stackUsageRow) int {
	if a.imageCount != b.imageCount {
		if a.imageCount > b.imageCount {
			return -1
		}
		return 1
	}
	if c := strings.Compare(a.key.typeKey, b.key.typeKey); c != 0 {
		return c
	}
	if c := stackBasisRank(a.key.basis) - stackBasisRank(b.key.basis); c != 0 {
		return c
	}
	if a.key.basis == communityStackBasisModelVersionID {
		switch {
		case a.key.id < b.key.id:
			return -1
		case a.key.id > b.key.id:
			return 1
		default:
			return 0
		}
	}
	return strings.Compare(a.key.str, b.key.str)
}

// stackBasisRank fixes the identity-basis tie-break precedence:
// modelVersionId, then hash, then name.
func stackBasisRank(basis string) int {
	switch basis {
	case communityStackBasisModelVersionID:
		return 0
	case communityStackBasisHash:
		return 1
	default:
		return 2
	}
}

// topRows splits the sorted identity rows into the fixed top and the
// omitted-identity count. Omitted image counts are never summed because
// one image can use several omitted identities.
func (g *stackGroupAccumulator) topRows() (top []stackUsageRow, otherIdentityCount int) {
	rows := g.rows()
	n := min(communityStackTopLimit, len(rows))
	return rows[:n], len(rows) - n
}

// resourceSummary projects the full resource block with non-nil top arrays
// in every group, including for an empty sample.
func (acc *communityStackAccumulator) resourceSummary() CommunityResourceSummary {
	return CommunityResourceSummary{
		EntryCount:             acc.entryCount,
		IdentifiedEntryCount:   acc.identifiedEntryCount,
		UnidentifiedEntryCount: acc.entryCount - acc.identifiedEntryCount,
		ImagesWithEntries:      len(acc.imagesWithEntries),
		Checkpoints:            acc.resourceGroup(stackGroupCheckpoints),
		LoRAs:                  acc.loraGroup(),
		OtherResources:         acc.resourceGroup(stackGroupOther),
	}
}

// resourceGroup projects one non-LoRA group.
func (acc *communityStackAccumulator) resourceGroup(kind stackGroupKind) CommunityResourceGroup {
	group := acc.groups[kind]
	rows, other := group.topRows()
	top := make([]CommunityResourceUsage, 0, len(rows))
	for _, row := range rows {
		top = append(top, CommunityResourceUsage{Identity: row.key.identity(), ImageCount: row.imageCount})
	}
	return CommunityResourceGroup{
		EntryCount:           group.entryCount,
		IdentifiedImageCount: len(group.identifiedImages),
		Top:                  top,
		OtherIdentityCount:   other,
	}
}

// loraGroup projects the LoRA group, attaching weight evidence to every
// emitted identity. An identity with no valid weight gets a null median
// and zero coverage rather than an invented default.
func (acc *communityStackAccumulator) loraGroup() CommunityLoRAGroup {
	group := acc.groups[stackGroupLoRAs]
	rows, other := group.topRows()
	top := make([]CommunityLoRAUsage, 0, len(rows))
	for _, row := range rows {
		top = append(top, CommunityLoRAUsage{
			Identity:   row.key.identity(),
			ImageCount: row.imageCount,
			Weight:     acc.weightSummary(row.key),
		})
	}
	return CommunityLoRAGroup{
		EntryCount:           group.entryCount,
		IdentifiedImageCount: len(group.identifiedImages),
		Top:                  top,
		OtherIdentityCount:   other,
	}
}

// weightSummary computes one identity's weight coverage and median from
// the retained per-image samples. Missing, malformed, or non-finite
// weights never reach the accumulator, so they reduce coverage instead of
// becoming samples.
func (acc *communityStackAccumulator) weightSummary(key stackIdentityKey) CommunityWeightSummary {
	perImage := acc.loraWeights[key]
	samples := make([]float64, 0, len(perImage))
	for _, weights := range perImage {
		for weight := range weights {
			samples = append(samples, weight)
		}
	}
	sort.Float64s(samples)
	return CommunityWeightSummary{
		Median:           communityStackMedian(samples),
		ImagesWithWeight: len(perImage),
		SampleCount:      len(samples),
	}
}

// communityStackMedian returns the median of an ascending sorted sample:
// the middle value for an odd count, the overflow-safe half-sum of the two
// middle values for an even count, and null for no samples.
func communityStackMedian(sorted []float64) *float64 {
	n := len(sorted)
	if n == 0 {
		return nil
	}
	if n%2 == 1 {
		v := sorted[n/2]
		return &v
	}
	v := sorted[n/2-1]/2 + sorted[n/2]/2
	return &v
}

// stackValueKind selects the value domain and tie-break of one parameter
// distribution.
type stackValueKind int

const (
	stackValueSteps stackValueKind = iota
	stackValueCFGScale
	stackValueSampler
	stackValueScheduler
)

// stackValueRow is one parameter value's distinct-image count before
// projection.
type stackValueRow struct {
	value      any
	imageCount int
}

// communityStackDistribution aggregates one parameter over the sample.
// Only the meta-origin observation with the matching name is read; values
// are never inferred from prompts, dimensions, seeds, defaults, or other
// images. Each sampled image contributes at most one value, so Top plus
// Other reconciles exactly to ObservedImages.
func communityStackDistribution(observations []CommunityObservation, name string, kind stackValueKind, sampled int) CommunityValueDistribution {
	counts := map[any]int{}
	for _, obs := range observations {
		for _, param := range obs.Parameters {
			if param.Origin != communityOriginMeta || param.Name != name {
				continue
			}
			if value, ok := communityStackParameterValue(param.Value, kind); ok {
				counts[value]++
			}
		}
	}

	rows := make([]stackValueRow, 0, len(counts))
	observed := 0
	for value, count := range counts {
		rows = append(rows, stackValueRow{value: value, imageCount: count})
		observed += count
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].imageCount != rows[j].imageCount {
			return rows[i].imageCount > rows[j].imageCount
		}
		return compareStackValues(rows[i].value, rows[j].value, kind) < 0
	})

	dist := CommunityValueDistribution{
		ObservedImages: observed,
		SampledImages:  sampled,
		Top:            make([]CommunityValueCount, 0, min(communityStackTopLimit, len(rows))),
	}
	n := min(communityStackTopLimit, len(rows))
	for _, row := range rows[:n] {
		dist.Top = append(dist.Top, CommunityValueCount{Value: row.value, ImageCount: row.imageCount})
	}
	for _, row := range rows[n:] {
		dist.Other.ImageCount += row.imageCount
		dist.Other.DistinctValueCount++
	}
	return dist
}

// communityStackParameterValue decides whether one normalized observation
// is a valid observation for the parameter: steps accept any normalized
// int64 (including zero and unusual negatives), cfgScale accepts finite
// floats with signed zero canonicalized, and sampler/scheduler accept
// strings whose trimmed form is non-empty while retaining the exact
// original string (no case folding, trimming, or alias merging).
func communityStackParameterValue(value any, kind stackValueKind) (any, bool) {
	switch kind {
	case stackValueSteps:
		v, ok := value.(int64)
		if !ok {
			return nil, false
		}
		return v, true
	case stackValueCFGScale:
		v, ok := value.(float64)
		if !ok {
			return nil, false
		}
		return canonicalizeCommunityStackFloat(v), true
	default:
		s, ok := value.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, false
		}
		return s, true
	}
}

// compareStackValues breaks distribution ties by numeric value ascending
// for steps/cfgScale or exact bytewise string ascending for
// sampler/scheduler. Values are unique map keys, so the order is total.
func compareStackValues(a, b any, kind stackValueKind) int {
	switch kind {
	case stackValueSteps:
		av, bv := a.(int64), b.(int64)
		switch {
		case av < bv:
			return -1
		case av > bv:
			return 1
		default:
			return 0
		}
	case stackValueCFGScale:
		av, bv := a.(float64), b.(float64)
		switch {
		case av < bv:
			return -1
		case av > bv:
			return 1
		default:
			return 0
		}
	default:
		return strings.Compare(a.(string), b.(string))
	}
}

// communityStackTextAvailability classifies prompt and negative-prompt
// availability for every sampled item from the same bounded body. Only
// statuses are counted; decoded strings are discarded immediately.
func communityStackTextAvailability(items []json.RawMessage, returned int) CommunityTextAvailability {
	var availability CommunityTextAvailability
	for _, item := range items {
		promptStatus, negativeStatus := communityStackItemTextStatus(item)
		availability.PromptStatus.add(promptStatus)
		availability.NegativePromptStatus.add(negativeStatus)
	}
	availability.PromptStatus.finishRate(returned)
	availability.NegativePromptStatus.finishRate(returned)
	return availability
}

// communityStackItemTextStatus classifies one raw item: a non-object item
// is notProvided for both fields, absent/null metadata is notProvided, an
// unsupported metadata shape is unsupported for both fields, and a
// metadata object classifies each field independently through the existing
// text normalizer (whitespace-only strings stay provided).
func communityStackItemTextStatus(raw json.RawMessage) (string, string) {
	if classifyRecipeRaw(raw) != recipeRawObject {
		return recipeStatusNotProvided, recipeStatusNotProvided
	}
	var src communitySourceImage
	if err := json.Unmarshal(raw, &src); err != nil {
		return recipeStatusNotProvided, recipeStatusNotProvided
	}
	meta, hasMeta := decodeCommunityMeta(src.Meta)
	if !hasMeta {
		if communityMetadataStatus(src.Meta) == recipeStatusUnsupported {
			return recipeStatusUnsupported, recipeStatusUnsupported
		}
		return recipeStatusNotProvided, recipeStatusNotProvided
	}
	promptStatus, _ := recipeTextValue(meta.Prompt)
	negativeStatus, _ := recipeTextValue(meta.NegativePrompt)
	return promptStatus, negativeStatus
}
