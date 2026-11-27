package cmd

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"civitai-cli/internal/api"
	"civitai-cli/internal/report"
)

// Latest-published evaluation state tokens. They are the stable English
// vocabulary of the additive normal-JSON latestPublished block and the
// human LATEST line; scripts key on them, so renaming is a contract
// change, not a refactor. The tokens are deliberately informational: no
// outdated/upgrade/recommended/best wording exists here.
const (
	// Latest status values (vetLatestPublished.Status).
	vetLatestStatusKnown   = "known"
	vetLatestStatusTied    = "tied"
	vetLatestStatusUnknown = "unknown"

	// Latest-level reason values (vetLatestPublished.Reason). "none" and
	// "latestTied" are shared with the comparison vocabulary below.
	vetReasonNone                 = "none"
	vetReasonLatestTied           = "latestTied"
	vetReasonModelUnavailable     = "modelUnavailable"
	vetReasonVersionsUnavailable  = "versionsUnavailable"
	vetReasonNoVisibleVersions    = "noVisibleVersions"
	vetReasonIdentityUnavailable  = "versionIdentityUnavailable"
	vetReasonTimestampUnavailable = "timestampUnavailable"

	// Current-to-latest relationship values (vetLatestComparison.Status).
	vetRelationCurrentLatest  = "currentLatest"
	vetRelationNewerPublished = "newerPublished"
	vetRelationTiedLatest     = "tiedLatest"
	vetRelationUnknown        = "unknown"

	// Relationship reason values (vetLatestComparison.Reason).
	vetReasonLatestUnavailable  = "latestUnavailable"
	vetReasonCurrentNotVisible  = "currentVersionNotVisible"
	vetReasonOrderIndeterminate = "timestampOrderIndeterminate"

	// Exact base-model change values (vetLatestComparison.BaseModelChange).
	vetBaseChangeChanged   = "changed"
	vetBaseChangeUnchanged = "unchanged"
	vetBaseChangeUnknown   = "unknown"
)

// vetLatestVersion carries the unique latest-published visible version's
// source facts. Name and BaseModel are null when the source value is
// empty; PublishedAt keeps the original RFC3339 source string (parsing is
// for comparison only, never normalization).
type vetLatestVersion struct {
	ID          int     `json:"id"`
	Name        *string `json:"name"`
	PublishedAt string  `json:"publishedAt"`
	BaseModel   *string `json:"baseModel"`
}

// vetLatestComparison relates the vetted (by-hash) version to the latest
// result and carries the independent exact base-model change. Status and
// Reason follow the fixed table in the task design: relationship unknowns
// never erase independently known latest identity or base facts.
type vetLatestComparison struct {
	Status          string `json:"status"`
	Reason          string `json:"reason"`
	BaseModelChange string `json:"baseModelChange"`
}

// vetLatestPublished is the additive latestPublished block of the normal
// vet JSON report (and the data source of the human LATEST line). Every
// key is always emitted: Version is an object only for a unique known
// latest (null otherwise), and TiedVersionIDs is always a JSON array —
// empty for known/unknown, ascending unique IDs for a tied maximum.
type vetLatestPublished struct {
	Status         string              `json:"status"`
	Reason         string              `json:"reason"`
	Version        *vetLatestVersion   `json:"version"`
	TiedVersionIDs []int               `json:"tiedVersionIds"`
	Comparison     vetLatestComparison `json:"comparison"`
}

// deriveVetLatestPublished evaluates the latest-published visible-version
// context of a vet hit from the authoritative by-hash current version and
// the optional owning-model response. It is pure — no HTTP, filesystem,
// clock, logging, or rendering — so the human and JSON views consume one
// shared, deterministic result.
//
// Evidence rules: every entry of the embedded modelVersions array counts
// as visible (no status/availability filtering, no wall clock). The
// latest is the unique maximum parsed RFC3339 instant — never response
// order, IDs, names, or download counts. A definitive latest claim
// requires complete evidence: positive unique IDs and a valid timestamp
// on EVERY visible entry, because an entry with unknown publication time
// could be the actual maximum; incomplete evidence degrades the whole
// claim to unknown instead of selecting from the remaining subset.
// Multiple maxima are reported as a sorted tie rather than resolved by
// any invented tie-breaker.
func deriveVetLatestPublished(current *api.ModelVersion, model *api.Model) vetLatestPublished {
	latest := vetLatestPublished{
		Status:         vetLatestStatusUnknown,
		TiedVersionIDs: []int{},
		Comparison: vetLatestComparison{
			Status:          vetRelationUnknown,
			Reason:          vetReasonLatestUnavailable,
			BaseModelChange: vetBaseChangeUnknown,
		},
	}

	// Fixed latest-level precedence: the first evidence gap decides the
	// reason, so identical inputs always produce the identical state.
	if model == nil {
		latest.Reason = vetReasonModelUnavailable
		return latest
	}
	versions := model.ModelVersions
	if versions == nil {
		latest.Reason = vetReasonVersionsUnavailable
		return latest
	}
	if len(versions) == 0 {
		latest.Reason = vetReasonNoVisibleVersions
		return latest
	}

	// Identity evidence: one unambiguous positive ID per visible entry.
	// Duplicates fail here rather than masquerading as a maximum tie.
	seen := make(map[int]bool, len(versions))
	for _, v := range versions {
		if v.ID <= 0 || seen[v.ID] {
			latest.Reason = vetReasonIdentityUnavailable
			return latest
		}
		seen[v.ID] = true
	}

	// Timestamp evidence: complete or nothing. An unparseable (including
	// empty) timestamp on any entry — maximum candidate or not — makes
	// the whole latest claim unknown instead of being skipped.
	instants := make(map[int]time.Time, len(versions))
	var maxInstant time.Time
	for _, v := range versions {
		ts, err := time.Parse(time.RFC3339, v.PublishedAt)
		if err != nil {
			latest.Reason = vetReasonTimestampUnavailable
			return latest
		}
		instants[v.ID] = ts
		if maxInstant.IsZero() || ts.After(maxInstant) {
			maxInstant = ts
		}
	}

	// Maximum candidates by parsed instant; equivalent offsets of the
	// same instant tie. Sorting keeps the tied ID array deterministic
	// regardless of response order.
	maxIDs := make([]int, 0, 1)
	for _, v := range versions {
		if instants[v.ID].Equal(maxInstant) {
			maxIDs = append(maxIDs, v.ID)
		}
	}
	slices.Sort(maxIDs)

	if len(maxIDs) > 1 {
		latest.Status = vetLatestStatusTied
		latest.Reason = vetReasonLatestTied
		latest.TiedVersionIDs = maxIDs
	} else {
		latestID := maxIDs[0]
		for i := range versions {
			if versions[i].ID != latestID {
				continue
			}
			latest.Status = vetLatestStatusKnown
			latest.Reason = vetReasonNone
			latest.Version = &vetLatestVersion{
				ID:          versions[i].ID,
				Name:        vetNullableString(versions[i].Name),
				PublishedAt: versions[i].PublishedAt,
				BaseModel:   vetNullableString(versions[i].BaseModel),
			}
			break
		}
	}

	latest.Comparison = vetCompareToLatest(latest, current, instants, maxInstant)
	return latest
}

// vetCompareToLatest derives the current-to-latest relationship and the
// independent exact base-model change once the latest result is known or
// tied. The relationship uses only the visible-list snapshot (the
// embedded current entry's instant, never the standalone by-hash
// publishedAt, so both operands come from one response). The base change
// compares the authoritative by-hash current base with the unique latest
// base as exact case-sensitive strings and still runs when the current
// version is not visible, because base equality is not a chronology
// claim. Ties never select a candidate for comparison.
func vetCompareToLatest(latest vetLatestPublished, current *api.ModelVersion, instants map[int]time.Time, maxInstant time.Time) vetLatestComparison {
	cmp := vetLatestComparison{
		Status:          vetRelationUnknown,
		BaseModelChange: vetBaseChangeUnknown,
	}

	curID, curBase := 0, ""
	if current != nil {
		curID, curBase = current.ID, current.BaseModel
	}

	if latest.Status == vetLatestStatusKnown && latest.Version != nil &&
		curBase != "" && latest.Version.BaseModel != nil && *latest.Version.BaseModel != "" {
		if curBase == *latest.Version.BaseModel {
			cmp.BaseModelChange = vetBaseChangeUnchanged
		} else {
			cmp.BaseModelChange = vetBaseChangeChanged
		}
	}

	curInstant, curVisible := time.Time{}, false
	if curID > 0 {
		if ts, ok := instants[curID]; ok {
			curInstant, curVisible = ts, true
		}
	}
	if !curVisible {
		cmp.Reason = vetReasonCurrentNotVisible
		return cmp
	}

	switch latest.Status {
	case vetLatestStatusTied:
		cmp.Reason = vetReasonLatestTied
		if slices.Contains(latest.TiedVersionIDs, curID) {
			cmp.Status = vetRelationTiedLatest
		}
	case vetLatestStatusKnown:
		latestID := 0
		if latest.Version != nil {
			latestID = latest.Version.ID
		}
		switch {
		case latestID == curID:
			cmp.Status = vetRelationCurrentLatest
			cmp.Reason = vetReasonNone
		case maxInstant.After(curInstant):
			cmp.Status = vetRelationNewerPublished
			cmp.Reason = vetReasonNone
		default:
			// Defensive residual: unreachable after valid maximum
			// evaluation (a different visible current entry cannot
			// postdate the unique maximum), kept so malformed evidence
			// never becomes a chronology claim.
			cmp.Reason = vetReasonOrderIndeterminate
		}
	}
	return cmp
}

// vetLatestLineText formats the value part of the one-line human LATEST
// summary (the caller prepends the 10-column "LATEST" key). Known latest
// prints name, full numeric ID, YYYY-MM-DD date, base, relationship, and
// base-change state; tied/unknown latest prints no candidate facts, only
// the sorted tie IDs or a concise reason. Only the display name is
// eligible for console-width truncation — IDs, timestamps, statuses,
// reasons, and base values are never abbreviated.
func vetLatestLineText(latest vetLatestPublished, width int) string {
	relation := vetRelationText(latest.Comparison)
	baseChange := vetBaseChangeText(latest.Comparison.BaseModelChange)
	switch latest.Status {
	case vetLatestStatusKnown:
		name, base := "unknown", "unknown"
		if latest.Version.Name != nil && *latest.Version.Name != "" {
			name = *latest.Version.Name
		}
		if latest.Version.BaseModel != nil && *latest.Version.BaseModel != "" {
			base = *latest.Version.BaseModel
		}
		date := report.DateText(latest.Version.PublishedAt)
		fixed := len(" (id ") + len(strconv.Itoa(latest.Version.ID)) + len(", ") + len(date) + len(")") +
			len("   base: ") + len(base) + len("   relation: ") + len(relation) + len("; ") + len(baseChange)
		return fmt.Sprintf("%s (id %d, %s)   base: %s   relation: %s; %s",
			report.Truncate(name, max(width-10-fixed, 12)), latest.Version.ID, date, base, relation, baseChange)
	case vetLatestStatusTied:
		return fmt.Sprintf("unknown (latest tied; version ids %s)   relation: %s; %s",
			vetJoinIDs(latest.TiedVersionIDs), relation, baseChange)
	default:
		return fmt.Sprintf("unknown (%s)   relation: %s; %s",
			vetLatestReasonText(latest.Reason), relation, baseChange)
	}
}

// vetRelationText maps the machine relationship tokens to their human
// wording. An unknown relationship appends its specific reason when the
// latest identity itself is known or tied; a latest-level unknown already
// carries its reason in the LATEST value, so latestUnavailable stays bare
// rather than being duplicated.
func vetRelationText(c vetLatestComparison) string {
	switch c.Status {
	case vetRelationCurrentLatest:
		return "current latest"
	case vetRelationNewerPublished:
		return "newer published"
	case vetRelationTiedLatest:
		return "tied latest"
	}
	switch c.Reason {
	case vetReasonCurrentNotVisible:
		return "unknown (current version not visible)"
	case vetReasonLatestTied:
		return "unknown (latest tied)"
	case vetReasonOrderIndeterminate:
		return "unknown (timestamp order indeterminate)"
	default:
		return "unknown"
	}
}

// vetBaseChangeText maps the machine base-change tokens to their human
// wording.
func vetBaseChangeText(change string) string {
	switch change {
	case vetBaseChangeChanged:
		return "base changed"
	case vetBaseChangeUnchanged:
		return "base unchanged"
	default:
		return "base change unknown"
	}
}

// vetLatestReasonText maps latest-level unknown reasons to the concise
// human wording used inside the LATEST value.
func vetLatestReasonText(reason string) string {
	switch reason {
	case vetReasonModelUnavailable:
		return "model details unavailable"
	case vetReasonVersionsUnavailable:
		return "version list unavailable"
	case vetReasonNoVisibleVersions:
		return "no visible versions"
	case vetReasonIdentityUnavailable:
		return "version identities invalid"
	case vetReasonTimestampUnavailable:
		return "timestamps unavailable or invalid"
	default:
		return "evidence unavailable"
	}
}

// vetNullableString converts an empty source value to JSON null so absent
// latest name/base facts stay explicit instead of rendering as "".
func vetNullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// vetJoinIDs renders the sorted tied version IDs comma-separated (no
// spaces), matching the human tie wording "version ids 10,20".
func vetJoinIDs(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ",")
}
