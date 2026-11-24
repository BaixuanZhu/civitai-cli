# Design: bounded community recipe sampling

Status: **Design converged for implementation review; implementation not started or authorized.**

## 1. Architecture and ownership

The feature is a source adapter, not a second recipe engine. The parent-approved invariant is absolute: every list or summary omits large fields; large fields such as prompts and raw/free-form metadata appear only in an explicitly selected single-image detail view.

It has four deliberately small boundaries:

| Boundary | Responsibility | Must not do |
|---|---|---|
| Civitai `/images` endpoint | Build one bounded list query or, after explicit selection, one exact single-image detail query through `api.Client.Get`; decode responses tolerantly | Pagination, bulk detail, rendering, coverage policy, CDN traffic |
| Pure community projection | Convert browse items into compact observations, coverage, and resource stacks | HTTP, CLI flags, recipe-core selection, list detail maps, persistence |
| Recipe-core adapter | Map the reviewed source-neutral contract; expose large fields only through explicit single-detail selection | Redefine recipe semantics, return detail arrays, or bypass core output contracts |
| Command/report surface | Parse reviewed options, orchestrate official/community sources, render public DTOs | Large fields in any list/summary, raw-meta list JSON, direct HTTP |

Preferred endpoint placement is `internal/api/images.go` with browse-specific response types beside existing API types. The pure projector should be a child-owned module whose final package/file placement is confirmed during recipe-core review: either a clearly separated `internal/recipe/community*` source file or a small `internal/community` package imported by `cmd -> community -> api`. It must not import the recipe core, which preserves fixture-level independence and avoids a cycle.

The adapter and public recipe command wiring follow the sibling core's reviewed ownership. Existing `show`/`vet` prompt code is not repurposed: it has different provenance, coverage, detail, and resource requirements.

## 2. Endpoint contract

### 2.1 List input and query

A logical community list sample accepts a positive version ID, limit 1-100 (default 20), and one of two sort options (default `Most Reactions`, fixed `period=AllTime`). The approved source selector is `civ recipe <modelVersionId> --source community`; default `official` sends no `/images` request. Combining official and community sources in one view is deferred.

| Query key | Value |
|---|---|
| `modelVersionId` | Exact positive version ID |
| `withMeta` | `true` |
| `type` | `image` |
| `limit` | Default 20 or explicit value, 1-100 |
| `sort` | `Most Reactions` (default) or `Newest` |
| `period` | `AllTime` |

Use `url.Values.Encode()` through `Client.Get`; spaces in `Most Reactions` must be encoded. Do not send `modelId`, `page`, `cursor`, `nsfw`, `browsingLevel`, or resource-resolution parameters.

One list sample means one logical endpoint operation. Existing transient retries are retained; no application-level alternate request, continuation, fan-out, fallback, or automatic detail fetch is allowed. A continuation in a successful response is reduced to `continuationOffered=true`; the opaque token and next-page URL are not exposed.

### 2.2 Approved explicit single-detail command and request

The approved detail UX is `civ recipe image <imageId> [--json]`. It performs one logical `GET /images` with exactly `imageId=<positive id>`, `withMeta=true`, `type=image`, and `limit=1`. It sends no list sort/period, `modelVersionId`, `modelId`, `page`, `cursor`, `nsfw`, or `browsingLevel`. Existing transient-retry policy applies, but there is no application-level retry with another ID or filter.

This request is independent of community listing and is never fanned out across list items. A successful detail response must contain exactly one item with the requested ID; zero, multiple, or mismatched items produce a detail-unavailable result without cursor traversal, broader filters, or claims about deletion versus visibility.

The command returns only the selected image's detail, including the large fields the reviewed core chooses to expose. It does not load a prior list, reuse cross-command memory, write detail state, or claim prior-list query provenance. Each invocation's response data is released after rendering.

### 2.3 Browse response DTO

Use a browse-specific type rather than extending the embedded-version `api.Image` contract. Important differences are image/post/user provenance, textual NSFW level, reaction stats, `modelVersionIds`, and free-form browse metadata.

The envelope contains `items` plus continuation metadata. Each item retains:

- scalar provenance: `id`, `postId`, `username`, `createdAt`, `type`, `baseModel`;
- `nsfwLevel` and `browsingLevel` as returned visibility facts;
- top-level `width`, `height`, and `modelVersionIds` without treating them as a typed resource stack;
- `stats` with independently optional reaction/comment counts;
- `meta` as `json.RawMessage`, preserving absent/null/object distinctions and avoiding a failed whole-response decode when one free-form field drifts.

The item CDN `url`, `hash` BlurHash, and thumbnail fields may be decoded only if needed by core detail; the sampler never requests them. Every list/summary omits media URLs and BlurHash.

Continuation metadata is read only for presence. A valid envelope with no `items` decodes as an empty, non-nil summary slice at the public boundary.

## 3. Internal source contract

Names below are illustrative until recipe-core review; semantics are binding unless a review topic changes them.

```text
CommunitySample
  query: requested version, limit, sort, period=AllTime, type=image
  coverage: requestedLimit, returned, withMeta, usable, continuationOffered
  observations: []CommunityObservation       // public-safe compact projection only

CommunityObservation
  source: "community-images"
  imageID, postID, username, createdAt, pageURL, upstreamPosition
  visibility: nsfwLevel, browsingLevel
  reactions: optional like/dislike/heart/laugh/cry/comment counts
  parameters: []ObservedParameter
  resources: []ObservedResource

ObservedParameter
  name, normalizedValue, origin(meta|top-level)

ObservedResource
  origin(civitaiResources|resources), stackPosition
  optional type, modelVersionID, name, hash, normalizedWeight

CommunityDetail                         // separate exact-image command only
  imageID + original free-form metadata needed by reviewed core detail view
```

`CommunitySample` contains no detail map or large-field payload. Its projector extracts compact observations and releases raw list metadata after the invocation. The exact-image response is projected independently into one `CommunityDetail`; it neither reads nor writes list state. Keeping list observations and detail results as different types makes large-field redaction structural rather than a rendering convention.

No global registry, cross-command handoff, detail store, or mutable package state is needed. Construction and clock/ID concerns stay injectable through ordinary function parameters if required by the core; do not add a new package-level test global.

## 4. Projection rules

### 4.1 Free-form metadata

Interpret `meta` only when it is a non-null JSON object. Decode known fields independently from raw messages so one bad field cannot reject sibling fields or items. Unknown keys are ignored by list projection; they become visible only if independently returned by the exact-image detail command and exposed by the core.

A supported scalar parameter is a present JSON string, number, or boolean with these rules:

- null is absent evidence and remains null;
- an empty/whitespace-only string is absent evidence for community coverage and is not converted to zero;
- arrays and objects do not count as usable scalar observations or enter list output; they produce null plus issue and are never coerced to strings;
- numeric zero is valid observed evidence;
- known numeric strings normalize through the approved shared R7 normalizer, so `"20"` and `20` have the same normalized value; invalid numeric text produces null plus issue and no unit/default inference occurs;
- seed remains an exact decimal string to preserve precision.

MVP scalar allowlist: `seed`, `steps`, `cfgScale`, `sampler`, `scheduler`, `clipSkip`, `denoisingStrength`, `width`, and `height`. Prompt and negativePrompt are never summary parameters. Top-level dimensions use origin `top-level`; if both metadata and top-level dimensions exist, neither silently overwrites the other.

Normalized values provide deterministic type/value equality for output and any approved aggregation. Human rendering can display normalized scalars without changing their JSON representation.

### 4.2 Resources

Read `civitaiResources` and `resources` independently. Each array entry keeps its origin and original position. Known identity/type/weight fields are parsed independently; unknown fields and unknown resource `type` strings do not invalidate the entry.

Identity evidence means at least one non-empty `modelVersionId`, `name`, or `hash`. Weight alone is not identity. Explicit weights use the approved R7 normalizer: numeric strings become JSON numbers, invalid/non-scalar values become null plus issue, and there is no default or prompt-derived weight.

The observed resource list is the concatenation of the two origin-labeled arrays in metadata order, with no sorting, deduplication, name resolution, or implicit insertion of the requested version. Top-level `modelVersionIds` is provenance only: it cannot reconstruct entry types, weights, duplicates, or order and therefore cannot make an item usable by itself.

Malformed resource entries do not abort the sample. They cannot contribute identity or usability unless their readable fields satisfy the rules above. Full malformed/raw metadata remains eligible only for explicit core detail, not summary speculation.

### 4.3 Coverage

Compute coverage from the returned items after tolerant projection:

```text
returned = len(items)
withMeta = count(meta is a non-null object)
usable   = count(has supported scalar observation OR has identified resource entry)
```

`{}` counts toward withMeta but not usable. Prompt-only and unknown-field-only objects behave the same. An identified resource counts even when no scalar generation setting is present. Coverage denominators are always `returned`; an optional `usable/withMeta` ratio must be separately labeled and never replace `usable/returned`.

For zero returned items, counts are zero and ratios are null in JSON or `n/a` in text. `continuationOffered` is independent of counts: an empty response with a cursor still reports the offered continuation but does not follow it.

## 5. Summary, detail, and optional aggregation

### 5.1 List and summary redaction

Human list output begins with source/query and coverage, then per-image observations. JSON is a normalized public DTO, not the upstream body. Arrays serialize as `[]`, absent optional scalar values are omitted or null consistently, and no list detail map/raw meta is reachable through JSON tags.

Every list and summary renderer—human or JSON—omits prompt excerpts/keys, negative prompts, raw/free-form metadata, full workflows, and equivalent large fields. Tests search both serialized keys and unique values to catch accidental raw embedding in every output mode.

Reaction counts remain separate. There is no total-reaction field unless a future reviewer explicitly requests a transparent arithmetic convenience; no such field is needed for this MVP. No quality language or recommendation field is generated.

### 5.2 Approved core-owned single detail

The approved `civ recipe image <imageId> [--json]` command performs the bounded exact-image request in §2.2 and renders only the matching item's detail. The recipe core owns prompts/full-metadata presentation; the community list surface has no detail-selection flag, prompt-bearing list mode, or bulk detail output.

The detail command is independent of any earlier list. It does not load saved list output, reuse cross-command memory, or persist its response. Its provenance is solely the exact-image response, not an assumed earlier list query. Response data is released after the current detail view is rendered.

A detail miss/mismatch is a core detail-unavailable outcome, not a reason to broaden the sample, page, or fetch another ID. Source labels and image IDs prevent confusion with official examples or community list positions.

### 5.3 Aggregation

The MVP emits per-image observations in upstream order only. It does not produce frequency tables, averages, stacks summaries, rankings, quality scores, or recommended values. This avoids inventing equivalence rules for free-form data and keeps every value auditable.

Any future aggregation requires a separate approved contract covering normalized-value equality, exact resource-stack equivalence, deterministic order, counts, supporting image IDs, provenance, and large-field redaction. It is not part of this task.

## 6. Data flow and failure boundaries

```text
reviewed list options
  -> validate locally (no HTTP)
  -> api.ListImages(exact bounded modelVersionId query)
  -> decode browse envelope
  -> pure compact per-item projection + coverage
  -> CommunitySample {public compact observations only}
  -> reviewed recipe-core adapter
       -> every list/summary stays large-field-free
       -> release list response data; no detail handoff

independent approved detail invocation

civ recipe image <imageId> [--json]
  -> validate locally (no HTTP)
  -> api.GetImageDetail(imageId, withMeta=true, type=image, limit=1)
  -> require exactly one matching item
  -> reviewed recipe-core single-detail view
       -> large fields for this selected image only
       -> no list-state input, cursor, fan-out, handoff, or persistence
```

- Flag/ID/limit/sort errors happen before client construction or request execution and map to exit 1.
- Existing auth, 404, deterministic API, and exhausted transient network errors flow through `execute` unchanged. No special community "not found" exit is introduced.
- Invalid envelope JSON is a decode error. Item-level free-form drift follows projection degradation and does not fabricate coverage.
- Empty list items is a successful sample. A message may note sparse metadata/visibility and the bounded query, but must not claim the version is missing or automatically try Newest/Most Reactions alternately.
- Empty, multiple, or mismatched detail items is detail-unavailable; visibility versus absence remains unknown and no broader request follows.
- If full metadata exceeds the existing 5 MB client body cap, retain the cap and return the existing error with guidance to use a smaller list limit. Do not raise the cap, stream to disk, or page automatically in this task.

## 7. Compatibility, privacy, and operational behavior

- Official-only recipe behavior is the compatibility baseline. Community code is unreachable unless selected.
- Existing commands and their raw/decoded JSON shapes remain untouched.
- Existing bearer token, proxy, timeout, User-Agent, retry, stdout/stderr, and exit-code behavior is reused. Query/provenance output never contains tokens or authorization headers.
- The server remains the sole enforcer of NSFW/account visibility. The client sends no elevation override and makes no attempt to recover omitted images.
- No sample data or detail metadata is written to config, cache, logs, or a recipe store. List response data is released after compact rendering, and detail response data is released after its invocation. Debug logging can contain the bounded API URL but not response metadata/prompts.
- No third-party module, persistence migration, completion-time network call, cross-command handoff, or background goroutine is introduced.

## 8. Tradeoffs

- **One list response versus representativeness:** one list response is predictable and non-crawler-like, but sort/visibility bias is unavoidable. Coverage labels and provenance mitigate interpretation risk; pagination would not belong to this task.
- **Independent detail request versus list-only evidence:** the approved detail command costs one exact-image request, but keeps list commands stateless and makes the large-field boundary explicit. The command receives no list state and returns only the requested image's detail.
- **Small default versus response cost:** full metadata responses can be large even though summaries redact prompts. The approved default 20 is conservative; larger limits remain explicit user choices, and the client never silently fetches more.
- **Normalized scalars versus raw-type fidelity:** the approved shared normalizer treats numeric strings and numbers equivalently in known fields and keeps seed as a decimal string. This loses the original number-versus-string distinction but gives official/community recipes one contract and simpler comparison/aggregation; invalid shapes remain explicit null-plus-issue states.
- **Two resource arrays versus one invented schema:** origin-labeled projection preserves more evidence without claiming all tools mean exactly the same thing.
- **Separate source DTO versus direct core types:** a small adapter adds mapping code but makes this child testable without the core and prevents an unreviewed core contract from leaking into the API layer.
- **Public/private DTO split versus `omitempty`:** separate types are slightly more code but structurally prevent accidental prompt serialization.

## 9. Rollback and rollout

Roll out only after the parent convergence review and explicit implementation approval. First land/test endpoint and pure projection independently, then the thin adapter and reviewed list/detail surface, then documentation/live integration.

Rollback removes the recipe-source registration/adapter and command surface; no data migration or persisted state exists. Independently tested endpoint/projection code can remain unused temporarily or be reverted as one unit. Existing official commands require no compatibility rollback.

## 10. Approved decisions

- **R1 CLI:** `civ recipe <modelVersionId> --source official|community [--json]`; default official performs no `/images` call. Combined official+community is deferred. `civ recipe image <imageId>` is the independent detail command.
- **R2 default limit:** 20 within 1–100; detail always uses `limit=1`.
- **R3 default sort:** Most Reactions with fixed AllTime; Newest is supported. Neither implies quality.
- **R4 aggregation:** per-image observations only for the MVP; any future aggregation needs a separate approved contract.
- **R5 core contract:** community registers an optional source beneath the recipe surface, reuses its summary/detail DTOs and shared normalizer, and does not create a competing recipe engine. Final package placement is settled during integration.
- **R6 independent detail:** exact `imageId+withMeta=true+type=image+limit=1`, no list-state handoff, cursor, or persistence.
- **R7 scalar normalization:** known numeric strings become numbers, seed stays decimal string, invalid/non-scalar values produce null plus issue, and unknown fields/units are not inferred.
