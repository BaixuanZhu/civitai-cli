package cmd

import (
	"fmt"
	"io"
	"strings"

	"civitai-cli/internal/api"
)

// renderCommunityStackSummary writes the aggregate-only community stack
// view: exact query, provenance, and coverage first, then caveats, text
// availability, resource identity groups, and parameter distributions.
// The renderer receives only the aggregate DTO, so per-image observations,
// prompt text, raw metadata, workflows, media/page locations, and
// continuation values are unreachable by construction.
func renderCommunityStackSummary(w io.Writer, s *api.CommunityStackSummary) error {
	fmt.Fprintf(w, "COMMUNITY STACK  version %d   sort %s   period %s   limit %d\n",
		s.Query.VersionID, s.Query.Sort, s.Query.Period, s.Query.Limit)
	fmt.Fprintf(w, "SOURCE     %s   %s\n", s.Provenance.Source, s.Provenance.APIPath)

	continuation := "no"
	if s.Coverage.ContinuationOffered {
		continuation = "yes (not followed)"
	}
	fmt.Fprintf(w, "COVERAGE   returned %d of %d requested   with meta %s   usable %s   continuation %s\n",
		s.Coverage.Returned, s.Coverage.RequestedLimit,
		communityCoverageText(s.Coverage.WithMeta, s.Coverage.Returned),
		communityCoverageText(s.Coverage.Usable, s.Coverage.Returned),
		continuation)

	fmt.Fprintln(w, "CAVEATS    bounded non-random sample   continuation not followed   descriptive evidence only")
	fmt.Fprintln(w, "           Most Reactions and Newest are biased discovery orders, not random samples;")
	fmt.Fprintln(w, "           counts and medians are never recommendations, quality scores, or reproducibility guarantees.")

	writeCommunityTextAvailability(w, "PROMPT AVAILABILITY", s.TextAvailability.PromptStatus, s.Coverage.Returned)
	writeCommunityTextAvailability(w, "NEGATIVE PROMPT AVAILABILITY", s.TextAvailability.NegativePromptStatus, s.Coverage.Returned)

	fmt.Fprintf(w, "\nRESOURCES   entries %d   identified %d   unidentified %d   images with entries %d\n",
		s.Resources.EntryCount, s.Resources.IdentifiedEntryCount,
		s.Resources.UnidentifiedEntryCount, s.Resources.ImagesWithEntries)
	writeCommunityResourceGroup(w, "CHECKPOINTS", s.Resources.Checkpoints, false)
	writeCommunityLoRAGroup(w, "LORAS", s.Resources.LoRAs)
	writeCommunityResourceGroup(w, "OTHER RESOURCES", s.Resources.OtherResources, true)

	fmt.Fprintln(w, "\nPARAMETERS")
	writeCommunityParameterDistribution(w, "steps", s.Parameters.Steps)
	writeCommunityParameterDistribution(w, "cfgScale", s.Parameters.CFGScale)
	writeCommunityParameterDistribution(w, "sampler", s.Parameters.Sampler)
	writeCommunityParameterDistribution(w, "scheduler", s.Parameters.Scheduler)
	return nil
}

// writeCommunityTextAvailability prints one aggregate text-availability
// block: the provided share of returned (or n/a at a zero denominator) and
// all four status counts. No text value exists in this view.
func writeCommunityTextAvailability(w io.Writer, label string, status api.CommunityTextStatusCoverage, returned int) {
	fmt.Fprintf(w, "\n%s   provided %s   empty %d   notProvided %d   unsupportedJsonShape %d\n",
		label, communityCoverageText(status.Provided, returned),
		status.Empty, status.NotProvided, status.UnsupportedJSONShape)
}

// writeCommunityResourceGroup prints one checkpoint/other resource group:
// entry and identified-image coverage, at most five identity rows with
// their distinct-image counts, and the omitted-identity count. Omitted
// image counts are never summed because identity image sets can overlap.
func writeCommunityResourceGroup(w io.Writer, label string, group api.CommunityResourceGroup, withType bool) {
	fmt.Fprintf(w, "\n%s   entries %d   identified images %d\n", label, group.EntryCount, group.IdentifiedImageCount)
	if len(group.Top) == 0 {
		fmt.Fprintln(w, "  (none observed)")
	}
	for _, usage := range group.Top {
		fmt.Fprintf(w, "  %s   images %d\n", communityStackIdentityText(usage.Identity, withType), usage.ImageCount)
	}
	if group.OtherIdentityCount > 0 {
		fmt.Fprintf(w, "  (%d more identities omitted)\n", group.OtherIdentityCount)
	}
}

// writeCommunityLoRAGroup prints the LoRA group: like the plain resource
// groups, plus each identity's weight median and valid-weight coverage
// (images with a valid weight over the identity's distinct images, and the
// retained per-image sample count). A null median renders as "-".
func writeCommunityLoRAGroup(w io.Writer, label string, group api.CommunityLoRAGroup) {
	fmt.Fprintf(w, "\n%s   entries %d   identified images %d\n", label, group.EntryCount, group.IdentifiedImageCount)
	if len(group.Top) == 0 {
		fmt.Fprintln(w, "  (none observed)")
	}
	for _, usage := range group.Top {
		fmt.Fprintf(w, "  %s   images %d   median %s   weights %d/%d images   samples %d\n",
			communityStackIdentityText(usage.Identity, false), usage.ImageCount,
			recipeFloatValue(usage.Weight.Median),
			usage.Weight.ImagesWithWeight, usage.ImageCount, usage.Weight.SampleCount)
	}
	if group.OtherIdentityCount > 0 {
		fmt.Fprintf(w, "  (%d more identities omitted)\n", group.OtherIdentityCount)
	}
}

// communityStackIdentityText renders one conservative identity: the type
// label only where it discriminates (the other-resources group, with "-"
// for untyped entries), then the selected basis value. Only the basis
// field is ever shown; secondary labels are not part of the identity.
func communityStackIdentityText(id api.CommunityResourceIdentity, withType bool) string {
	parts := make([]string, 0, 2)
	if withType {
		parts = append(parts, "type "+recipeHumanText(id.Type))
	}
	switch id.Basis {
	case "modelVersionId":
		parts = append(parts, "modelVersionId "+recipeIntValue(id.ModelVersionID))
	case "hash":
		parts = append(parts, "hash "+recipeHumanText(id.Hash))
	default:
		parts = append(parts, "name "+recipeHumanText(id.Name))
	}
	return strings.Join(parts, "   ")
}

// writeCommunityParameterDistribution prints one parameter's observed over
// sampled coverage, up to five value rows with their distinct-image
// counts, and the reconciling other line (images and distinct values
// outside the top). Empty observations get an explicit marker, never a
// fabricated row.
func writeCommunityParameterDistribution(w io.Writer, name string, dist api.CommunityValueDistribution) {
	fmt.Fprintf(w, "  %-9s observed %s\n", name, communityCoverageText(dist.ObservedImages, dist.SampledImages))
	if len(dist.Top) == 0 {
		fmt.Fprintln(w, "    (none observed)")
	}
	for _, row := range dist.Top {
		fmt.Fprintf(w, "    %s   %d images\n", communityParameterText(row.Value), row.ImageCount)
	}
	fmt.Fprintf(w, "    other %d images   %d distinct values\n", dist.Other.ImageCount, dist.Other.DistinctValueCount)
}
