package resolver

import (
	"fmt"
)

// PolicyVersionV1 is the reviewed policy version compiled into this
// binary. A policy change that alters compatibility outcomes must bump it
// so machine reports stay interpretable.
const PolicyVersionV1 = "resolver-v1"

// Stable rule identifiers. Every compatibility decision and anchor
// finding carries one, so consumers can key on the rule rather than on
// message text.
const (
	// RuleAnchorCheckpoint is the single-anchor role rule (missing or
	// conflicting Checkpoint findings).
	RuleAnchorCheckpoint = "role.anchor.checkpoint"
	// RuleCompatibleBaseModel proves a dependent compatible: both sides
	// carry the same non-empty baseModel, and baseModelType matches when
	// both sides provide it.
	RuleCompatibleBaseModel = "compat.base-model.exact"
	// RuleConflictBaseModel proves a reviewed known conflict: both sides
	// carry non-empty, unequal baseModel values.
	RuleConflictBaseModel = "compat.base-model.conflict"
	// RuleConflictBaseModelType proves a reviewed known conflict: the
	// baseModel matches but both sides carry different non-empty
	// baseModelType values.
	RuleConflictBaseModelType = "compat.base-model-type.conflict"
	// RuleUnknownMissingBaseModel marks a tuple that cannot be decided
	// because one side has no baseModel metadata.
	RuleUnknownMissingBaseModel = "compat.base-model.missing"
	// RuleUnknownModelType marks a resource whose registry model type is
	// not a reviewed dependent role.
	RuleUnknownModelType = "compat.model-type.unknown"
	// RuleNotEvaluated marks a dependent whose identity did not resolve,
	// so no compatibility metadata existed to evaluate.
	RuleNotEvaluated = "compat.not-evaluated"
)

// DefaultPolicy returns the reviewed resolver-v1 policy: exactly one
// Checkpoint anchor; LORA, TextualInversion, Controlnet, and VAE are the
// known dependents; compatibility requires exact baseModel equality (and
// exact baseModelType equality when both sides provide one). No aliases,
// case folding, family inference, or baseModelWhitelist reuse is active.
func DefaultPolicy() Policy {
	return Policy{
		Version:        PolicyVersionV1,
		AnchorType:     "Checkpoint",
		DependentTypes: []string{"LORA", "TextualInversion", "Controlnet", "VAE"},
	}
}

// isAnchorType reports whether modelType is the policy's anchor role.
func (p Policy) isAnchorType(modelType string) bool {
	return modelType != "" && modelType == p.AnchorType
}

// isDependentType reports whether modelType is a reviewed dependent role.
// Unknown or empty types are deliberately not dependents: they fall
// through to an unknown compatibility outcome.
func (p Policy) isDependentType(modelType string) bool {
	for _, t := range p.DependentTypes {
		if t == modelType {
			return true
		}
	}
	return false
}

// sideOf projects a resource into the observable side of a compatibility
// result. It never invents metadata: unresolved resources keep zero ids
// and empty base fields.
func sideOf(index int, r ResourceResult) CompatSide {
	return CompatSide{
		Index:         index,
		ModelID:       r.Identity.ModelID,
		VersionID:     r.Identity.VersionID,
		ModelType:     r.Identity.ModelType,
		BaseModel:     r.Identity.BaseModel,
		BaseModelType: r.Identity.BaseModelType,
	}
}

// evaluateCompatibility applies the policy to every non-anchor resource
// in input order. It returns one result per evaluated resource plus the
// blocking findings for unknown and conflicting tuples. A dependent whose
// identity is unresolved yields not_evaluated (its own resource finding
// explains why); no compatibility finding is duplicated for it.
func (p Policy) evaluateCompatibility(results []ResourceResult, anchorIndex int) ([]CompatibilityResult, []Finding) {
	compat := make([]CompatibilityResult, 0, len(results))
	findings := make([]Finding, 0)
	anchor := results[anchorIndex]
	left := sideOf(anchorIndex, anchor)

	for i := range results {
		if i == anchorIndex {
			continue
		}
		r := results[i]
		right := sideOf(i, r)
		result := CompatibilityResult{
			LeftIndex:  anchorIndex,
			RightIndex: i,
			Status:     StatusUnknown,
			RuleID:     RuleNotEvaluated,
			Left:       left,
			Right:      right,
		}

		if r.Resolution != ResolutionResolved {
			result.Reason = "resource identity is unresolved; compatibility cannot be evaluated"
			compat = append(compat, result)
			continue
		}
		if p.isAnchorType(r.Identity.ModelType) {
			// A second resolved Checkpoint with the anchor's identity is
			// idempotent (no compatibility pair); a distinct one was already
			// reported as conflicting_anchor, so no single anchor exists.
			continue
		}
		if !p.isDependentType(r.Identity.ModelType) {
			result.RuleID = RuleUnknownModelType
			result.Reason = fmt.Sprintf("model type %q is not a reviewed dependent role", r.Identity.ModelType)
			compat = append(compat, result)
			findings = append(findings, p.compatibilityFinding(CodeUnknownCompatibility, i, result,
				fmt.Sprintf("model type %q has no reviewed compatibility rule", r.Identity.ModelType)))
			continue
		}

		p.applyBaseModelRules(&result)
		compat = append(compat, result)
		switch result.Status {
		case StatusConflict:
			findings = append(findings, p.compatibilityFinding(CodeBaseModelConflict, i, result, result.Reason))
		case StatusUnknown:
			findings = append(findings, p.compatibilityFinding(CodeUnknownCompatibility, i, result, result.Reason))
		}
	}
	return compat, findings
}

// applyBaseModelRules fills result for a known dependent: exact baseModel
// equality is required; baseModelType must also match when both sides
// provide it; missing base metadata is unknown, never compatible.
func (p Policy) applyBaseModelRules(result *CompatibilityResult) {
	leftBase := result.Left.BaseModel
	rightBase := result.Right.BaseModel
	leftType := result.Left.BaseModelType
	rightType := result.Right.BaseModelType

	switch {
	case leftBase == "" || rightBase == "":
		result.Status = StatusUnknown
		result.RuleID = RuleUnknownMissingBaseModel
		result.Reason = "baseModel metadata is missing on at least one side"
	case leftBase != rightBase:
		result.Status = StatusConflict
		result.RuleID = RuleConflictBaseModel
		result.Reason = fmt.Sprintf("baseModel %q does not match the checkpoint's %q", rightBase, leftBase)
	case leftType != "" && rightType != "" && leftType != rightType:
		result.Status = StatusConflict
		result.RuleID = RuleConflictBaseModelType
		result.Reason = fmt.Sprintf("baseModelType %q does not match the checkpoint's %q", rightType, leftType)
	default:
		result.Status = StatusCompatible
		result.RuleID = RuleCompatibleBaseModel
		result.Reason = "baseModel metadata matches the checkpoint"
	}
}

// compatibilityFinding builds the blocking finding for a compatibility
// decision, carrying both observed sides and the rule id.
func (p Policy) compatibilityFinding(code string, rightIndex int, result CompatibilityResult, message string) Finding {
	left := result.Left
	right := result.Right
	f := newFinding(code, []int{result.LeftIndex, rightIndex}, message, Evidence{Left: &left, Right: &right})
	f.RuleID = result.RuleID
	return f
}
