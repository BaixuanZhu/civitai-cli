#!/usr/bin/env bash
# Online integration smoke test for civ (civitai-cli).
#
# Runs the real binary against the live Civitai API (anonymous) and asserts
# exit codes plus --json response shapes for every command, including the
# local-only skill/login/logout commands. This is the canary for upstream API
# drift: the fixture-based unit tests stay green when Civitai changes a field
# or enum, only a live call catches that.
#
# Isolation: CIV_NO_BOOTSTRAP=1 and CIV_NO_CACHE=1 are exported unconditionally,
# so the binary never writes the user PATH/registry (even a release-flavored
# exe) and never serves a warm hash cache. USERPROFILE/HOME/APPDATA/LOCALAPPDATA
# are redirected to repo-local scratch dirs so skill install and login/logout
# never touch the real platform skill roots or token config. The only files
# written are repo-local scratch dirs, removed on exit; nothing outside the
# repo is modified.
#
# Coverage notes (honest limits of a live, anonymous, file-less CI runner):
#   - sync found/dupe and exit-code 2 (network) are covered by offline unit
#     tests (httptest); a live runner has no registered model bytes to hash and
#     no CLI base-url override to inject network faults, so they are not here.
#   - unbootstrap (the internal PATH-removal hook) is never run: it mutates
#     the real user PATH and is not part of the documented command surface.
#   - upgrade is asserted graceful (exit 0/1, no panic) and run LAST: with no
#     release it reports 404 (exit 1); after a release it may self-update the
#     test exe, which is safe only because nothing runs after it.
#   - login live coverage is rejection and usage only (invalid token via every
#     input form -> exit 4, usage errors -> exit 1); the success save path
#     needs a real account token and stays in the offline unit tests.
#
# Usage:
#   bash scripts/integration-test.sh [exe]
#   Default exe: dist/amd64/civ.exe (build it first: make build).
#
# Paths are repo-relative on purpose: Git Bash /tmp mounts are not resolvable
# by the native Windows exe or by Windows python, so scratch dirs live under
# the working directory (the repo root) where all three agree.

set -euo pipefail

EXE="${1:-dist/amd64/civ.exe}"

if [ ! -f "$EXE" ]; then
  echo "[integration] exe not found: $EXE (build it first: make build)" >&2
  exit 1
fi
if ! command -v python >/dev/null 2>&1; then
  echo "[integration] python is required for the JSON assertions" >&2
  exit 1
fi
if ! command -v cygpath >/dev/null 2>&1; then
  echo "[integration] cygpath (Git Bash) is required to isolate the Windows home" >&2
  exit 1
fi

# Hard isolation: never touch PATH/registry, never serve a cached response.
export CIV_NO_BOOTSTRAP=1
export CIV_NO_CACHE=1

WORK=".integration-work"
MODELS=".integration-models"
ISO=".integration-isolate"
rm -rf "$WORK" "$MODELS" "$ISO"
mkdir -p "$WORK" "$MODELS" "$ISO/home" "$ISO/appdata" "$ISO/localappdata"
trap 'rm -rf "$WORK" "$MODELS" "$ISO"' EXIT

# Redirect the Windows user dirs to scratch so skill/login/logout stay local.
export USERPROFILE="$(cygpath -w "$PWD/$ISO/home")"
export HOME="$PWD/$ISO/home"
export APPDATA="$(cygpath -w "$PWD/$ISO/appdata")"
export LOCALAPPDATA="$(cygpath -w "$PWD/$ISO/localappdata")"
ISO_HOME="$ISO/home"   # repo-relative view for the python filesystem assertions

# Two synthetic local files for the sync not-found path (anonymous by-hash 404).
printf 'integration-a' > "$MODELS/f1.safetensors"
printf 'integration-bb' > "$MODELS/f2.ckpt"

# Known registered hash (Anima turbo-v1.1, plan appendix D) and its AutoV2.
ANIMA_SHA256="FBA11953276B57EDF59D1DC4F1857AC05AA079C56F982B4D7C20298D57D3F7EB"
ANIMA_AUTOV2="FBA1195327"

echo "[integration] exe: $EXE (live Civitai API, anonymous; isolated home)"

# run <label> <args...>: capture stdout/stderr/exit-code under set -e.
run() {
  local label="$1"; shift
  local got=0
  "$EXE" "$@" >"$WORK/$label.out" 2>"$WORK/$label.err" || got=$?
  printf '%s' "$got" > "$WORK/$label.exit"
  echo "[integration] ran $label (exit $got)"
}

# run_stdin <label> <stdin-text> <args...>: like run but feeds stdin (login).
run_stdin() {
  local label="$1"; local input="$2"; shift 2
  local got=0
  printf '%s\n' "$input" | "$EXE" "$@" >"$WORK/$label.out" 2>"$WORK/$label.err" || got=$?
  printf '%s' "$got" > "$WORK/$label.exit"
  echo "[integration] ran $label (exit $got)"
}

# --- read surface (live anonymous) ---
run version          version
run search_json      search anima --json --limit 3 --view full
run search_compact   search anima --json --limit 3
run search_table     search stocking --type lora --limit 5
run search_filters   search girl --base Illustrious --sort most-downloaded --period all --type lora --json --limit 3 --view full
# search summary empty page (task 11-27-search-base-model-summary): an
# unregistered base passes through with a stderr warning and yields a
# successful zero-item page with a zeroed summary.
run search_compact_empty search summary-empty --base __civ_search_summary_no_such_base__ --json --limit 1
# search non-TTY names (task 11-27-non-tty-search-names): `run` redirects
# stdout, so this pairs the real executable's non-TTY human path with the
# full-JSON response above (identical query parameters).
run search_names_table search anima --limit 3
run show_json        show 2458426 --json --view full
run show_compact     show 2458426 --json
run show_version     show 2458426 -v 3263843 --json --view full
run show_version_compact show 2458426 -v 3263843 --json
run show_prompts     show 2458426 -v 3263843 --prompts --json
run recipe_summary   recipe 3263843 --json
run recipe_detail    recipe 3263843 --example 1 --json
run recipe_community recipe 3263843 --source community --limit 5 --json
run recipe_community_newest recipe 3263843 --source community --sort newest --limit 5 --json
# community stack summary (task 11-27-community-stack-summary): opt-in
# aggregate-only view over the same single bounded /images response.
run recipe_community_stack recipe 3263843 --source community --stack-summary --limit 5 --json
run recipe_community_stack_newest recipe 3263843 --source community --stack-summary --sort newest --limit 5 --json
# --stack-summary is community-only and keeps pre-network usage validation.
run recipe_stack_official recipe 3263843 --stack-summary
run recipe_stack_bad_limit recipe 3263843 --source community --stack-summary --limit 0
run recipe_stack_selector_conflict recipe 3263843 --source community --stack-summary --example 1
run recipe_bad_id    recipe 0
run recipe_bad_count recipe 3263843 --examples 11
run recipe_select_conflict recipe 3263843 --examples 2 --example 0
run recipe_image_bad recipe image 0
run resolve_not_ready resolve 0000000000000000000000000000000000000000000000000000000000000000 --json
run resolve_bad_ref  resolve DEADBEEF
run compare_models   compare models 2458426 260267 --json
run compare_versions compare versions 3108589 3263843 --json
run compare_diff     compare versions 3108589 3263843 --diff
run compare_few      compare models 2458426
run compare_dupe     compare models 2458426 2458426
run tags_json        tags anima --json --limit 5
run vet_hit          vet "$ANIMA_SHA256" --json
run vet_autov2       vet "$ANIMA_AUTOV2"
run vet_prompts      vet "$ANIMA_SHA256" --prompts --json
run vet_miss         vet DEADBEEF00
run vet_badinput     vet DEADBEEF
run sync             sync "$MODELS" --format csv
run sync_json        sync "$MODELS" --json
run usage_error      search anima --limit 101
run help_cmd         help
run unknown_cmd      definitely-not-a-command
# --timeout flag plumbing: a 1ns deadline is always exceeded -> exit 2,
# independent of civitai reachability (fails before the network matters).
run search_timeout_bad  search anima --limit 1 --timeout 1ns

# --- per-command help pages (task 11-22: `civ <cmd> help` and `civ <cmd> -h`) ---
# Both entry points are intercepted in the runner before any prompt, network
# call, or config write, so these runs are offline-assertable; the only live
# case below is search_help_me (the keyword is literal: "help me" is a real
# query). upgrade's help tokens are routed by main (design D4) and asserted
# here — distinct from the real self-update run that stays LAST.
for c in search show recipe resolve compare vet sync tags login logout proxy skill upgrade completion; do
  run "help_${c}_kw"   "$c" help
  run "help_${c}_flag" "$c" -h
done
# Named help routing (task 11-23-help-routing): `civ help <name>` reaches the
# same curated page as `civ <name> help` (byte-compare asserted below),
# `civ help upgrade` reaches upgrade's own main-routed page, and unknown or
# extra-word targets are usage errors (exit 1). All offline: no page renders
# touch the network.
run help_search_named  help search
run help_recipe_named  help recipe
run help_resolve_named help resolve
run help_compare_named help compare
run help_upgrade_named help upgrade
run help_unknown       help nope
run help_extra         help search extra
# Shell completion (task 11-23-shell-completion): each supported shell prints
# its static script (exit 0), an unknown shell is a usage error (exit 1), and
# bare `civ completion` prints the command's help page (exit 0). All offline.
run completion_bash completion bash
run completion_zsh  completion zsh
run completion_fish completion fish
run completion_pwsh completion powershell
run completion_bad  completion tcsh
run completion_bare completion
# login help with empty stdin: interception happens before the prompt, so a
# regression to the interactive prompt would read the empty line and fail
# the exit-0 assertion loudly instead of hanging (login_* convention).
run_stdin help_login_stdin "" login help
run search_help_me search help me
run search_nope    search --nope

# --- token management (isolated config; anonymous + invalid token) ---
run logout           logout
run_stdin login_bad  "invalid_integration_token" login
# New login input forms (invalid token -> live 401 -> exit 4). Empty stdin on
# the flag/positional/global forms is deliberate: a regression to the
# interactive prompt would read the empty line and exit 1, failing the
# exit-4 assertion loudly instead of hanging the script.
run_stdin login_pos_bad     "" login invalid_integration_token
run_stdin login_key_bad     "" login --key invalid_integration_token
run_stdin login_t_bad       "" login -t invalid_integration_token
run_stdin login_gtoken_bad  "" login --token invalid_integration_token
run_stdin login_keydash_bad "invalid_integration_token" login --key -
# Offline usage errors (no probe, no config write) -> exit 1.
run login_multi  login a b
run login_empty  login --key ""

# --- proxy subcommand (isolated config; pure local file ops) ---
# Config state is snapshotted per step so assertions see each phase exactly;
# the section ends by removing the config again (unset keeps the file), which
# restores the no-config end state the login checks above rely on.
# search_proxy_bad proves the --proxy flag actually routes traffic: a dead
# local port refuses the connection fast -> network error -> exit 2.
CFG="$ISO/appdata/civ/config.json"
# snap_cfg <label>: copy the isolated config for the python assertions
# (an empty snapshot file means "no config.json at that point").
snap_cfg() {
  if [ -f "$CFG" ]; then cp "$CFG" "$WORK/$1.json"; else : > "$WORK/$1.json"; fi
}
run proxy_status_none  proxy
run proxy_set_bad      proxy set not-a-url
snap_cfg proxy_after_setbad
run proxy_set          proxy set http://127.0.0.1:7890
snap_cfg proxy_after_set
run proxy_get          proxy get
run proxy_get_json     proxy get --json
run proxy_get_verbose  proxy get --verbose
run proxy_unset        proxy unset
snap_cfg proxy_after_unset
run proxy_unset_again  proxy unset
run proxy_usage        proxy set
run search_proxy_bad   search anima --limit 1 --proxy http://127.0.0.1:9
# GitHub slot (task 11-23-github-proxy-slot): set/get/unset --github touch
# only the githubProxy key; the civitai slot survives the scoped unset. The
# civitai slot was cleared by proxy_unset above, so restore it first — this
# block needs a live civitai value to prove it survives `unset --github`.
run proxy_set_again proxy set http://127.0.0.1:7890
run proxy_set_gh    proxy set --github http://127.0.0.1:7891
snap_cfg proxy_after_setgh
run proxy_get_gh    proxy get
run proxy_unset_gh  proxy unset --github
snap_cfg proxy_after_unsetgh
rm -f "$CFG"

# --- skill subcommand (isolated home; pure local file ops) ---
# Install all three (claude+codex left installed for filesystem assertions;
# zcode removed to prove remove). --json captures paths/actions.
run skill_install    skill install --platform claude,zcode,codex --scope user --json
run skill_update     skill update --platform claude --scope user --json
run skill_remove     skill remove --platform zcode --scope user --json
run skill_bad        skill install --platform bogus
# Interactive fallback (task 11-22-skill-interactive-menu): piped stdin is
# not a terminal, so the text prompt flow runs — now without the removed
# Proceed confirmation. A separate isolated home with only .claude planted
# makes detection preselect exactly claude (the cases above left .zcode/
# .agents dirs in $ISO_HOME, which detection would otherwise pick up);
# the empty platform line accepts that preselection, "user" picks the scope.
INT_HOME="$ISO/home-interactive"
mkdir -p "$INT_HOME/.claude"
got=0
printf '\nuser\n' | HOME="$INT_HOME" USERPROFILE="$(cygpath -w "$PWD/$INT_HOME")" \
  "$EXE" skill install >"$WORK/skill_interactive.out" 2>"$WORK/skill_interactive.err" || got=$?
printf '%s' "$got" > "$WORK/skill_interactive.exit"
echo "[integration] ran skill_interactive (exit $got)"

# --- self-update: graceful handling, run LAST (may replace the test exe) ---
# --proxy to a refused port must fail fast (exit 1) before the release check
# — no network wait (task 11-23-upgrade-direct). The real upgrade run stays
# LAST.
run upgrade_proxy_dead upgrade --proxy http://127.0.0.1:1
run upgrade          upgrade

echo "[integration] asserting responses..."
python - "$WORK" "$ISO_HOME" "$INT_HOME" <<'PYEOF'
import json
import os
import re
import sys

WORK = sys.argv[1]
ISO_HOME = sys.argv[2]
INT_HOME = sys.argv[3]
failures = []


def check(cond, label):
    print(('  ok   ' if cond else '  FAIL ') + label)
    if not cond:
        failures.append(label)


def exit_of(label):
    with open(os.path.join(WORK, label + '.exit'), encoding='utf-8') as fh:
        return int(fh.read().strip())


def out_of(label):
    with open(os.path.join(WORK, label + '.out'), encoding='utf-8') as fh:
        return fh.read()


def err_of(label):
    with open(os.path.join(WORK, label + '.err'), encoding='utf-8') as fh:
        return fh.read()


def json_of(label):
    # An upstream outage that returns non-JSON FAILs cleanly here instead of
    # crashing the whole assertion run.
    try:
        return json.loads(out_of(label))
    except Exception as exc:
        check(False, '%s: stdout is valid JSON (%s)' % (label, exc))
        return None


# version
check(exit_of('version') == 0 and out_of('version').startswith('civ '),
      'version: exit 0 and prints "civ <version>"')

# search --json --view full: the top level stays the raw items array (never an
# items/summary envelope), with core fields on the first entries
raw_items = json_of('search_json')
check(exit_of('search_json') == 0 and isinstance(raw_items, list),
      'search --json --view full: top level remains a raw array, not an object')
items = raw_items if isinstance(raw_items, list) else []
check(len(items) > 0
      and all('id' in i and 'name' in i and 'type' in i for i in items[:3]),
      'search --json: non-empty items array carrying id/name/type')

# search table: human header present, plus the returned-page summary footer
# (task 11-27-search-base-model-summary); live base names are never pinned
table_out = out_of('search_table')
check(exit_of('search_table') == 0 and 'NAME' in table_out,
      'search table: exit 0 and header row present')
ret_m = re.search(r'^Returned: (\d+); multi-base models: \d+; models without base: \d+$',
                  table_out, re.M)
cov_m = re.search(r'^Base coverage: (.+)$', table_out, re.M)
check(ret_m is not None and cov_m is not None,
      'search table: Returned/Base coverage footer labels present')
check(cov_m is not None and '=0' not in cov_m.group(1),
      'search table: no fabricated zero-count base category')

# search non-TTY names (task 11-27-non-tty-search-names): every model name in
# the paired full-JSON response appears verbatim in the redirected human
# output. Guaranteed long/Unicode cases live in the unit tests; live results
# only provide the real-pipe smoke, so names are never hard-coded here.
names_out = out_of('search_names_table')
check(exit_of('search_names_table') == 0 and 'NAME' in names_out,
      'search non-TTY table: exit 0 and NAME header present')
for i in items:
    check(isinstance(i.get('name'), str) and i['name'] in names_out,
          'search non-TTY table: full name of model %s present verbatim' % i.get('id'))

# search filters: --base/--sort/--period/--type accepted, still a raw array
filt = json_of('search_filters')
check(exit_of('search_filters') == 0 and isinstance(filt, list)
      and (len(filt) == 0 or all('id' in i and 'name' in i for i in filt)),
      'search filters (--base/--sort/--period/--type): exit 0 and raw JSON array')

# show --json: structured model object
model = json_of('show_json')
check(exit_of('show_json') == 0 and isinstance(model, dict)
      and ('id' in model or 'name' in model),
      'show --json: exit 0 and model object with id/name')

# show version drill --json: parses (version detail object)
ver = json_of('show_version')
check(exit_of('show_version') == 0 and ver is not None,
      'show -v --json: exit 0 and valid JSON version detail')

# default compact contracts (task 11-24-search-show-compact, D1/D2): the
# default --json shape is a versioned envelope; --view full above proves the
# legacy shapes still work.
sc = json_of('search_compact')
check(exit_of('search_compact') == 0 and isinstance(sc, dict)
      and sc.get('schemaVersion') == 'civ.search/v1' and sc.get('kind') == 'modelSearch'
      and isinstance(sc.get('items'), list) and len(sc['items']) > 0
      and all('id' in i and 'name' in i for i in sc['items'][:3]),
      'search --json default compact: versioned envelope with model summaries')
check(sc is not None and 'description' not in json.dumps(sc),
      'search compact: no description blobs')

# search compact summary (task 11-27-search-base-model-summary): returned-page
# facts only. Coverage and both reconciliation counters are recomputed here
# from the items with the same evidence policy (usable model-level bases win;
# otherwise distinct bases across all embedded versions; exact per-model
# dedup), then compared against the emitted summary including its
# count-descending/name-ascending order. Live base names are never pinned.
summ = sc.get('summary') if isinstance(sc, dict) else None
check(isinstance(summ, dict)
      and summ.get('returned') == len(sc['items'])
      and isinstance(summ.get('baseCoverage'), list)
      and isinstance(summ.get('modelsWithMultipleBases'), int)
      and isinstance(summ.get('modelsWithoutBase'), int),
      'search compact: summary object with returned == len(items) and required members')


def summary_bases(item):
    bases = []
    for b in item.get('baseModels') or []:
        if isinstance(b, str) and b.strip() and b not in bases:
            bases.append(b)
    if bases:
        return bases
    for v in item.get('versions') or []:
        if not isinstance(v, dict):
            continue
        b = v.get('baseModel')
        if isinstance(b, str) and b.strip() and b not in bases:
            bases.append(b)
    return bases


expect_cov = {}
expect_multi = 0
expect_nobase = 0
for it in sc['items']:
    bs = summary_bases(it)
    if not bs:
        expect_nobase += 1
    if len(bs) > 1:
        expect_multi += 1
    for b in bs:
        expect_cov[b] = expect_cov.get(b, 0) + 1
expect_rows = sorted(expect_cov.items(), key=lambda kv: (-kv[1], kv[0]))
got_rows = None
if isinstance(summ, dict) and isinstance(summ.get('baseCoverage'), list):
    got_rows = [(r.get('baseModel'), r.get('models')) for r in summ['baseCoverage']]
check(got_rows == expect_rows
      and all(isinstance(m, int) and m > 0 for _, m in (got_rows or []))
      and isinstance(summ, dict)
      and summ.get('modelsWithMultipleBases') == expect_multi
      and summ.get('modelsWithoutBase') == expect_nobase,
      'search compact: baseCoverage matches independent recomputation, sorted count-desc/name-asc')

# search compact empty page: unregistered base -> exit 0, items [], zeroed
# summary; the unknown-base warning stays on stderr (stdout parsed as JSON above)
se = json_of('search_compact_empty')
check(exit_of('search_compact_empty') == 0 and isinstance(se, dict)
      and se.get('items') == []
      and isinstance(se.get('summary'), dict)
      and se['summary'].get('returned') == 0
      and se['summary'].get('baseCoverage') == []
      and se['summary'].get('modelsWithMultipleBases') == 0
      and se['summary'].get('modelsWithoutBase') == 0,
      'search compact empty: exit 0, items [], zeroed summary')

sm = json_of('show_compact')
check(exit_of('show_compact') == 0 and isinstance(sm, dict)
      and sm.get('schemaVersion') == 'civ.show/v1' and sm.get('kind') == 'modelSummary'
      and isinstance(sm.get('model'), dict),
      'show --json default compact: modelSummary envelope')

sv = json_of('show_version_compact')
check(exit_of('show_version_compact') == 0 and isinstance(sv, dict)
      and sv.get('kind') == 'versionSummary'
      and isinstance(sv.get('version'), dict)
      and isinstance(sv['version'].get('files'), list) and len(sv['version']['files']) > 0,
      'show -v --json default compact: versionSummary with files')

# show --prompts --json: recipe view with prompts array + skipped count
sp = json_of('show_prompts')
check(exit_of('show_prompts') == 0 and isinstance(sp, dict)
      and isinstance(sp.get('prompts'), list) and len(sp['prompts']) > 0
      and isinstance(sp.get('skipped'), int),
      'show -v --prompts --json: non-empty recipes + skipped count')

# tags --json: name array
tags = json_of('tags_json')
check(exit_of('tags_json') == 0 and isinstance(tags, list) and len(tags) > 0
      and all(isinstance(t, str) for t in tags),
      'tags --json: non-empty array of names')

# vet hit --json: report object with version block
report = json_of('vet_hit')
check(exit_of('vet_hit') == 0 and isinstance(report, dict) and 'version' in report,
      'vet known SHA256 --json: exit 0 and report carries version')

# vet latestPublished (task 11-27-vet-latest-version-context): additive block
# with approved vocabularies and internally consistent shapes. Live version
# drift is tolerated: known/tied/unknown are all valid outcomes.
lp = report.get('latestPublished') if isinstance(report, dict) else None
check(isinstance(lp, dict)
      and lp.get('status') in ('known', 'tied', 'unknown')
      and isinstance(lp.get('tiedVersionIds'), list)
      and isinstance(lp.get('comparison'), dict),
      'vet --json: latestPublished object with approved status vocabulary')
comp = lp.get('comparison') if isinstance(lp, dict) else None
check(isinstance(comp, dict)
      and comp.get('status') in ('currentLatest', 'newerPublished', 'tiedLatest', 'unknown')
      and comp.get('baseModelChange') in ('changed', 'unchanged', 'unknown')
      and (comp.get('status') != 'unknown' or bool(comp.get('reason'))),
      'vet --json: relationship/base tokens approved, unknown comparison has a reason')
vid = report.get('version', {}).get('id') if isinstance(report, dict) else None
lp_ok = False
if isinstance(lp, dict) and isinstance(comp, dict):
    st = lp.get('status')
    rel = comp.get('status')
    if st == 'known':
        v = lp.get('version') or {}
        lp_ok = (isinstance(v.get('id'), int) and v.get('id') > 0
                 and 'name' in v and 'publishedAt' in v and 'baseModel' in v
                 and rel in ('currentLatest', 'newerPublished', 'unknown')
                 and (rel != 'currentLatest' or v.get('id') == vid)
                 and (rel != 'newerPublished' or v.get('id') != vid))
    elif st == 'tied':
        ids = lp.get('tiedVersionIds')
        lp_ok = (lp.get('version') is None and isinstance(ids, list)
                 and len(ids) >= 2 and ids == sorted(set(ids))
                 and comp.get('baseModelChange') == 'unknown'
                 and (rel != 'tiedLatest' or vid in ids))
    else:  # unknown
        lp_ok = (lp.get('version') is None and bool(lp.get('reason'))
                 and rel == 'unknown' and comp.get('baseModelChange') == 'unknown')
check(lp_ok,
      'vet --json: latestPublished internal invariants per status shape')

# vet AutoV2 hit: human report carries exactly one LATEST line with state
# wording and no outdated/upgrade/recommendation language
av_lines = [ln for ln in out_of('vet_autov2').splitlines() if ln.startswith('LATEST')]
check(exit_of('vet_autov2') == 0 and len(av_lines) == 1
      and re.search(r'current latest|newer|tied|unknown', av_lines[0], re.I),
      'vet AutoV2: exit 0, exactly one LATEST line with state wording')
check(not av_lines
      or not re.search(r'outdated|upgrade|recommend|best', av_lines[0], re.I),
      'vet AutoV2: LATEST line carries no outdated/upgrade/recommendation wording')

# vet --prompts --json: focused recipe view, exactly {prompts,skipped} at the
# top level (no latestPublished or normal report keys)
vp = json_of('vet_prompts')
check(exit_of('vet_prompts') == 0 and isinstance(vp, dict)
      and isinstance(vp.get('prompts'), list) and len(vp['prompts']) > 0,
      'vet --prompts --json: non-empty recipes')
check(isinstance(vp, dict) and sorted(vp.keys()) == ['prompts', 'skipped'],
      'vet --prompts --json: exactly the focused {prompts,skipped} top-level keys')

# vet unknown hash -> exit 3 (not registered), no partial report on stdout
check(exit_of('vet_miss') == 3 and out_of('vet_miss').strip() == '',
      'vet unknown hash: exit 3 (not registered), empty stdout')

# vet bad input (8 hex -> path semantics, missing file) -> exit 1, empty stdout
check(exit_of('vet_badinput') == 1 and out_of('vet_badinput').strip() == '',
      'vet bad input: exit 1 (usage-class), empty stdout')

# sync: not_found data rows, overall exit 0
check(exit_of('sync') == 0 and 'not_found' in out_of('sync'),
      'sync synthetic dir: exit 0 with not_found rows')

# sync --json: array of not_found rows
sj = json_of('sync_json')
check(exit_of('sync_json') == 0 and isinstance(sj, list) and len(sj) > 0
      and all(r.get('status') == 'not_found' for r in sj),
      'sync --json: array of not_found rows')

# recipe (task 11-24-recipe-context): prompt-free bounded summary, singular
# explicit example detail, and pre-network usage errors.
rs = json_of('recipe_summary')
check(exit_of('recipe_summary') == 0 and isinstance(rs, dict)
      and rs.get('schemaVersion') == 'civ.recipe/v1' and rs.get('kind') == 'recipeSummary'
      and isinstance(rs.get('examples'), list) and 0 < len(rs['examples']) <= 3,
      'recipe --json: recipeSummary with bounded examples')
check(rs is not None and '"prompt"' not in out_of('recipe_summary')
      and '"negativePrompt"' not in out_of('recipe_summary')
      and 'description' not in json.dumps(rs),
      'recipe summary: prompt/negativePrompt/description absent')

rd = json_of('recipe_detail')
check(exit_of('recipe_detail') == 0 and isinstance(rd, dict)
      and rd.get('kind') == 'recipeExampleDetail'
      and isinstance(rd.get('example'), dict)
      and 'examples' not in rd,
      'recipe --example 0 --json: singular recipeExampleDetail')
check(isinstance(rd, dict)
      and isinstance(rd.get('example', {}).get('textParameters'), dict),
      'recipe detail: selected textParameters object present')

# community source: one bounded sample, per-image observations only, and
# large fields absent from the list shape.
rc = json_of('recipe_community')
check(exit_of('recipe_community') == 0 and isinstance(rc, dict)
      and rc.get('schemaVersion') == 'civ.recipe/v1'
      and rc.get('kind') == 'communityRecipeSample'
      and isinstance(rc.get('observations'), list)
      and isinstance(rc.get('coverage'), dict),
      'recipe --source community: bounded communityRecipeSample')
check(rc is not None and '"prompt"' not in out_of('recipe_community')
      and '"negativePrompt"' not in out_of('recipe_community'),
      'recipe community list: prompt/negativePrompt absent')
rcn = json_of('recipe_community_newest')
check(exit_of('recipe_community_newest') == 0 and isinstance(rcn, dict)
      and rcn.get('kind') == 'communityRecipeSample'
      and rcn.get('query', {}).get('sort') == 'Newest',
      'recipe --source community --sort Newest: accepted and reported')


# community stack summary (task 11-27-community-stack-summary): aggregate-only
# view over the same single bounded response. Live metadata content varies, so
# assertions pin structure, denominators, ordering, and redaction — never
# specific stack values, and never a retried sort to force data.
def walk_keys(obj, hit):
    if isinstance(obj, dict):
        for k, v in obj.items():
            hit.append(k)
            walk_keys(v, hit)
    elif isinstance(obj, list):
        for v in obj:
            walk_keys(v, hit)


def check_stack(label, want_sort):
    st = json_of(label)
    check(exit_of(label) == 0 and isinstance(st, dict)
          and st.get('schemaVersion') == 'civ.recipe/v1'
          and st.get('kind') == 'communityStackSummary'
          and st.get('topLimit') == 5
          and st.get('caveats') == ['boundedNonRandomSample',
                                    'continuationNotFollowed',
                                    'descriptiveEvidenceOnly']
          and not any(k in st for k in ('observations', 'examples', 'image', 'textParameters')),
          '%s: communityStackSummary envelope, fixed caveats, aggregate-only members' % label)
    if not isinstance(st, dict):
        return
    q = st.get('query') or {}
    cov = st.get('coverage') or {}
    prov = st.get('provenance') or {}
    check(q.get('versionId') == 3263843 and q.get('type') == 'image'
          and q.get('limit') == 5 and q.get('sort') == want_sort
          and q.get('period') == 'AllTime'
          and prov.get('source') == 'community-images'
          and prov.get('apiPath') == '/api/v1/images'
          and prov.get('versionId') == 3263843
          and sorted(cov.keys()) == ['continuationOffered', 'requestedLimit',
                                     'returned', 'usable', 'withMeta']
          and cov.get('requestedLimit') == 5
          and isinstance(cov.get('continuationOffered'), bool),
          '%s: query/provenance/coverage echo the one bounded request' % label)
    returned = cov.get('returned')
    res = st.get('resources') or {}
    groups_ok = True
    for gname in ('checkpoints', 'loras', 'otherResources'):
        grp = res.get(gname) or {}
        top = grp.get('top')
        if not isinstance(top, list) or len(top) > 5:
            groups_ok = False
            continue
        for row in top:
            ident = row.get('identity') or {}
            if ident.get('basis') not in ('modelVersionId', 'hash', 'name'):
                groups_ok = False
            if not isinstance(row.get('imageCount'), int) or row['imageCount'] < 0:
                groups_ok = False
            if gname != 'loras' and 'weight' in row:
                groups_ok = False
    check(groups_ok,
          '%s: resource groups bounded to top 5 with approved identity bases' % label)
    loras = (res.get('loras') or {}).get('top') or []
    check(all(((r.get('weight') or {}).get('sampleCount') == 0
               and (r.get('weight') or {}).get('median') is None)
              or (isinstance((r.get('weight') or {}).get('median'), (int, float))
                  and 0 <= ((r.get('weight') or {}).get('imagesWithWeight') or 0)
                  <= (r.get('imageCount') or 0))
              for r in loras),
          '%s: LoRA median null iff no valid weight samples, coverage bounded' % label)
    params = st.get('parameters') or {}
    param_ok = sorted(params.keys()) == ['cfgScale', 'sampler', 'scheduler', 'steps']
    for dist in params.values():
        top = (dist or {}).get('top') or []
        other = (dist or {}).get('other') or {}
        observed = (dist or {}).get('observedImages')
        if (dist or {}).get('sampledImages') != returned or len(top) > 5:
            param_ok = False
        if sum(r.get('imageCount') or 0 for r in top) + (other.get('imageCount') or 0) != observed:
            param_ok = False
    check(param_ok,
          '%s: four parameter distributions, sampled==returned, top+other==observed' % label)
    text = st.get('textAvailability') or {}
    text_ok = True
    for field in ('promptStatus', 'negativePromptStatus'):
        f = text.get(field) or {}
        counts = [f.get(k) for k in ('provided', 'empty', 'notProvided', 'unsupportedJsonShape')]
        if any(not isinstance(c, int) for c in counts) or sum(counts) != returned:
            text_ok = False
        rate = f.get('providedRate')
        if returned == 0 and rate is not None:
            text_ok = False
        if returned and isinstance(rate, (int, float)) \
                and abs(rate - (f.get('provided') or 0) / returned) > 1e-9:
            text_ok = False
    check(text_ok,
          '%s: text status counts sum to returned with exact provided rates' % label)
    keys = []
    walk_keys(st, keys)
    banned = {'prompt', 'negativeprompt', 'meta', 'workflow', 'comfyui', 'url',
              'pageurl', 'thumbnail', 'blurhash', 'cursor', 'continuationtoken',
              'nextpage', 'observations'}
    check(not banned.intersection(k.lower() for k in keys),
          '%s: no prompt/meta/workflow/media/cursor keys anywhere in the aggregate' % label)


check_stack('recipe_community_stack', 'Most Reactions')
check_stack('recipe_community_stack_newest', 'Newest')

for label in ('recipe_stack_official', 'recipe_stack_bad_limit', 'recipe_stack_selector_conflict'):
    check(exit_of(label) == 1 and out_of(label).strip() == '',
          '%s: exit 1, empty stdout (--stack-summary validation before network)' % label)

check(exit_of('recipe_bad_id') == 1 and out_of('recipe_bad_id').strip() == '',
      'recipe 0: exit 1, empty stdout')
check(exit_of('recipe_image_bad') == 1 and out_of('recipe_image_bad').strip() == '',
      'recipe image 0: exit 1, empty stdout')
check(exit_of('recipe_bad_count') == 1 and out_of('recipe_bad_count').strip() == '',
      'recipe --examples 11: exit 1, empty stdout')
check(exit_of('recipe_select_conflict') == 1 and out_of('recipe_select_conflict').strip() == '',
      'recipe --examples+--example: exit 1, empty stdout')

# resolve (task 11-24-resource-resolver): a valid but unregistered full SHA
# completes not-ready (exit 5) with the JSON report on stdout; a short hash is
# a usage error before any lookup.
rr = json_of('resolve_not_ready')
check(exit_of('resolve_not_ready') == 5 and isinstance(rr, dict)
      and rr.get('ready') is False,
      'resolve unknown full SHA: exit 5, JSON not-ready report on stdout')
check(exit_of('resolve_bad_ref') == 1 and out_of('resolve_bad_ref').strip() == '',
      'resolve DEADBEEF: exit 1 (short hash is usage), empty stdout')

# compare (task 11-24-model-version-compare): schema-1 model/version results,
# human --diff, and usage errors before any request.
cmm = json_of('compare_models')
check(exit_of('compare_models') == 0 and isinstance(cmm, dict)
      and cmm.get('schemaVersion') == 1 and cmm.get('kind') == 'model'
      and len(cmm.get('subjects', [])) == 2,
      'compare models --json: schema 1 model result with two subjects')
cmv = json_of('compare_versions')
check(exit_of('compare_versions') == 0 and isinstance(cmv, dict)
      and cmv.get('kind') == 'version' and len(cmv.get('subjects', [])) == 2,
      'compare versions --json: version result with two subjects')
check(exit_of('compare_diff') == 0 and out_of('compare_diff').strip() != '',
      'compare versions --diff: exit 0 with human diff output')
check(exit_of('compare_few') == 1 and out_of('compare_few').strip() == '',
      'compare with one subject: usage exit 1, empty stdout')
check(exit_of('compare_dupe') == 1 and out_of('compare_dupe').strip() == '',
      'compare with duplicate subjects: usage exit 1, empty stdout')

# usage error: limit over 100 rejected CLI-side -> exit 1, empty stdout
check(exit_of('usage_error') == 1 and out_of('usage_error').strip() == '',
      'search --limit 101: exit 1 (CLI-side cap), empty stdout')

# help / unknown command: usage surface (offline)
h = out_of('help_cmd')
check(exit_of('help_cmd') == 0 and 'Usage:' in h
      and 'proxy' in h and 'completion' in h,
      'civ help: exit 0, usage printed, proxy and completion listed')
heads = ['Discover:', 'Recipes:', 'Local models:', 'Utilities:']
positions = [h.find(x) for x in heads]
check(all(p >= 0 for p in positions) and positions == sorted(positions),
      'civ help: grouped headings Discover < Recipes < Local models < Utilities')
check(all(cmd in h for cmd in ('search', 'show', 'compare', 'tags', 'recipe',
                               'vet', 'sync', 'resolve', 'login', 'logout',
                               'proxy', 'skill', 'upgrade', 'completion',
                               'version', 'help')),
      'civ help: final command inventory present in the groups')
check('unbootstrap' not in h,
      'civ help: internal unbootstrap stays hidden')
check('civ <command> help' in out_of('help_cmd'),
      'civ help: contains the "Run civ <command> help" pointer (AC6)')
check(exit_of('unknown_cmd') == 1
      and 'unknown command' in err_of('unknown_cmd'),
      'unknown command: exit 1 (usage error)')

# per-command help pages (task 11-22, prd AC1/AC2): keyword and -h forms both
# exit 0, print the page on stdout (non-empty, Usage: present), keep stderr
# empty, and carry each command's signature content. Groups encode the
# "either form is fine" alternatives (show: --ver or -v; vet: Exit codes: or 3).
help_sigs = {
    'search': [['--sort'], ['Examples:']],
    'show': [['--ver', '-v']],
    'recipe': [['--examples'], ['--stack-summary'], ['Examples:']],
    'resolve': [['version:<id>'], ['Exit codes:']],
    'compare': [['--diff'], ['Examples:']],
    'vet': [['Exit codes:', '3']],
    'sync': [['--concurrency']],
    'tags': [['--limit']],
    'login': [['--key']],
    'logout': [['logout']],
    'proxy': [['socks5']],
    'skill': [['--platform']],
    'upgrade': [['upgrade']],
    'completion': [['bash']],
}
for cmd, groups in help_sigs.items():
    for form, token in (('kw', 'help'), ('flag', '-h')):
        label = 'help_%s_%s' % (cmd, form)
        out = out_of(label)
        check(exit_of(label) == 0 and out.strip() != '' and 'Usage:' in out
              and err_of(label) == '',
              'civ %s %s: exit 0, page on stdout, stderr empty' % (cmd, token))
    page = out_of('help_%s_kw' % cmd)
    missing = [grp for grp in groups if not any(tok in page for tok in grp)]
    check(not missing,
          'civ %s help: signature content present (missing: %s)' % (cmd, missing))

# login help with empty stdin: intercepted before the prompt -> exit 0
# immediately, no prompt text on stderr (anti-hang, prd AC3).
check(exit_of('help_login_stdin') == 0
      and 'Usage:' in out_of('help_login_stdin')
      and err_of('help_login_stdin') == '',
      'civ login help (empty stdin): exit 0 without hanging, no prompt')

# the keyword is literal-only: "help me" is a real (live) search query with
# ordinary search output (prd AC4).
check(exit_of('search_help_me') == 0 and out_of('search_help_me').strip() != '',
      'civ search help me: live query, exit 0 with results on stdout')

# unknown flag: exit 1, one-line error, Go default usage dump gone (AC5).
check(exit_of('search_nope') == 1
      and 'flag provided but not defined: -nope' in err_of('search_nope')
      and 'Usage of search' not in err_of('search_nope'),
      'civ search --nope: exit 1, one-line error, no default dump')

# uniform section order on a rendered page (prd AC8).
s = out_of('help_search_kw')
check(0 <= s.find('Usage:') < s.find('Options:') < s.find('Examples:')
      < s.find('Global flags'),
      'civ search help: sections ordered Usage < Options < Examples < Global flags')

# named help routing (task 11-23-help-routing): `civ help <name>` byte-equals
# `civ <name> help` for cmd-owned pages and for upgrade's main-routed page;
# unknown and extra-word targets exit 1 naming the target.
check(exit_of('help_search_named') == 0
      and out_of('help_search_named') == out_of('help_search_kw'),
      'civ help search: exit 0, byte-identical to `civ search help`')
for cmd in ['recipe', 'resolve', 'compare']:
    check(exit_of('help_%s_named' % cmd) == 0
          and out_of('help_%s_named' % cmd) == out_of('help_%s_kw' % cmd),
          'civ help %s: exit 0, byte-identical to `civ %s help`' % (cmd, cmd))
check(exit_of('help_upgrade_named') == 0
      and out_of('help_upgrade_named') == out_of('help_upgrade_kw'),
      'civ help upgrade: exit 0, byte-identical to `civ upgrade help`')
check(exit_of('help_unknown') == 1
      and 'unknown command' in err_of('help_unknown'),
      'civ help nope: exit 1 (unknown command)')
check(exit_of('help_extra') == 1
      and 'unknown command' in err_of('help_extra'),
      'civ help search extra: exit 1 (unknown command naming the full remainder)')

# shell completion (task 11-23-shell-completion): each supported shell prints
# its static script, tcsh is a usage error naming the valid shells, and the
# bare form prints the command's help page. The bash script anchors on
# complete -F and never completes the internal unbootstrap hook.
check(exit_of('completion_bash') == 0
      and 'complete -F _civ civ' in out_of('completion_bash')
      and 'unbootstrap' not in out_of('completion_bash'),
      'civ completion bash: exit 0, anchored script, no unbootstrap')
for label, shell in [('completion_zsh', 'zsh'), ('completion_fish', 'fish'),
                     ('completion_pwsh', 'powershell')]:
    check(exit_of(label) == 0 and out_of(label).strip() != '',
          'civ completion %s: exit 0, script printed' % shell)
check(exit_of('completion_bad') == 1
      and 'valid shells' in err_of('completion_bad'),
      'civ completion tcsh: exit 1 naming the valid shells')
check(exit_of('completion_bare') == 0
      and 'Usage:' in out_of('completion_bare'),
      'civ completion (bare): exit 0, help page printed')

# --timeout flag plumbing: 1ns deadline always exceeded -> exit 2
check(exit_of('search_timeout_bad') == 2,
      'search --timeout 1ns: exit 2 (deadline exceeded, flag applied)')

# logout: idempotent, no saved key in isolated config -> exit 0
check(exit_of('logout') == 0,
      'logout: exit 0 (idempotent, no saved key)')

# login with an invalid token: favorites probe 401 -> exit 4, no config saved
check(exit_of('login_bad') == 4,
      'login invalid token: exit 4 (rejected, no save)')
cfg = os.path.join(ISO_HOME, '..', 'appdata', 'civ', 'config.json')
check(not os.path.exists(cfg),
      'login invalid token: no config.json written')

# login new input forms with an invalid token: same live 401 -> exit 4, no
# interactive prompt (except --key -, which reads stdin by design), no key
# echo. Empty stdin on the flag/positional/global forms is deliberate: a
# regression to the prompt would read the empty line -> exit 1, failing
# the exit-4 check loudly, not hanging.
for label, form in [
    ('login_pos_bad', 'positional'),
    ('login_key_bad', '--key flag'),
    ('login_t_bad', '-t shorthand'),
    ('login_gtoken_bad', 'global --token'),
    ('login_keydash_bad', '--key - stdin read'),
]:
    check(exit_of(label) == 4
          and 'invalid_integration_token' not in out_of(label) + err_of(label),
          'login invalid token via %s: exit 4 (probe rejected, key not echoed)' % form)
    if label != 'login_keydash_bad':
        check('Paste API key' not in err_of(label),
              'login via %s: no interactive prompt' % form)
check(not os.path.exists(cfg),
      'login invalid token (all input forms): no config.json written')

# login offline usage errors: exit 1 with the usage message, no config write
check(exit_of('login_multi') == 1
      and 'at most one argument' in err_of('login_multi'),
      'login a b: exit 1 (usage error: at most one argument)')
check(exit_of('login_empty') == 1
      and 'no API key' in err_of('login_empty'),
      'login --key "": exit 1 (empty candidate)')
check(not os.path.exists(cfg),
      'login usage errors: no config.json written')

# proxy subcommand: local lifecycle (config snapshotted per step; the bash
# section restores the no-config end state the login checks rely on)
def cfg_snap(label):
    with open(os.path.join(WORK, label + '.json'), encoding='utf-8') as fh:
        body = fh.read().strip()
    return json.loads(body) if body else None

check(exit_of('proxy_status_none') == 0,
      'bare civ proxy: exit 0 (status; source is env-dependent)')
check(exit_of('proxy_set_bad') == 1,
      'proxy set not-a-url: exit 1 (validation, no save)')
check(cfg_snap('proxy_after_setbad') is None,
      'proxy set bad URL: no config.json written')
check(exit_of('proxy_set') == 0
      and cfg_snap('proxy_after_set') is not None
      and cfg_snap('proxy_after_set').get('proxy') == 'http://127.0.0.1:7890',
      'proxy set: exit 0, value persisted to config.json')
check(exit_of('proxy_get') == 0
      and 'http://127.0.0.1:7890' in out_of('proxy_get')
      and 'Source: config' in out_of('proxy_get'),
      'proxy get: exit 0, reports persisted value (source: config)')
gj = json_of('proxy_get_json')
check(exit_of('proxy_get_json') == 0 and isinstance(gj, dict)
      and gj.get('persisted') == 'http://127.0.0.1:7890'
      and gj.get('source') == 'config',
      'proxy get --json: {persisted, effective, source} shape')
check(exit_of('proxy_get_verbose') == 0
      and '[DEBUG]' in err_of('proxy_get_verbose')
      and 'http://127.0.0.1:7890' in err_of('proxy_get_verbose'),
      'proxy get --verbose: DEBUG line names the effective proxy')
snap = cfg_snap('proxy_after_unset')
check(exit_of('proxy_unset') == 0 and snap is not None
      and snap.get('proxy', '') == '' and snap.get('token', '') == '',
      'proxy unset: exit 0, proxy cleared, token untouched, file kept')
check(exit_of('proxy_unset_again') == 0,
      'proxy unset: idempotent (second unset exit 0)')
check(exit_of('proxy_usage') == 1,
      'proxy set (no URL): exit 1 (usage error)')
check(exit_of('search_proxy_bad') == 2,
      'search --proxy <dead port>: exit 2 (request routed through the flag proxy)')

# GitHub proxy slot (task 11-23-github-proxy-slot): set --github persists the
# githubProxy key alongside the civitai slot; get shows both scopes; the
# scoped unset clears only githubProxy and keeps the civitai slot.
snap = cfg_snap('proxy_after_setgh')
check(exit_of('proxy_set_gh') == 0 and snap is not None
      and snap.get('proxy') == 'http://127.0.0.1:7890'
      and snap.get('githubProxy') == 'http://127.0.0.1:7891',
      'proxy set --github: exit 0, both slots persisted (proxy + githubProxy)')
check(exit_of('proxy_get_gh') == 0
      and 'http://127.0.0.1:7890' in out_of('proxy_get_gh')
      and 'GitHub persisted: http://127.0.0.1:7891' in out_of('proxy_get_gh'),
      'proxy get: text report shows both scopes (incl. the github block)')
snap = cfg_snap('proxy_after_unsetgh')
check(exit_of('proxy_unset_gh') == 0 and snap is not None
      and snap.get('githubProxy', '') == ''
      and snap.get('proxy') == 'http://127.0.0.1:7890',
      'proxy unset --github: exit 0, github slot cleared, civitai slot kept')

# skill install: three platforms, JSON results, correct native roots
si = json_of('skill_install')
si = si if isinstance(si, list) else []
by_plat = {r.get('platform'): r for r in si}
check(exit_of('skill_install') == 0 and len(si) == 3
      and all(r.get('action') == 'installed' for r in si),
      'skill install: exit 0, three platforms installed')
claude_skill = os.path.join(ISO_HOME, '.claude', 'skills', 'civ-prompt', 'SKILL.md')
codex_skill = os.path.join(ISO_HOME, '.agents', 'skills', 'civ-prompt', 'SKILL.md')
zcode_skill = os.path.join(ISO_HOME, '.zcode', 'skills', 'civ-prompt')
check(os.path.isfile(claude_skill), 'skill install: claude SKILL.md materialized')
check(os.path.isfile(codex_skill), 'skill install: codex SKILL.md in .agents/skills')
check(not os.path.exists(os.path.join(ISO_HOME, '.codex')),
      'skill install: codex never writes .codex/')
# version placeholder rendered to the running civ version
if os.path.isfile(claude_skill):
    with open(claude_skill, encoding='utf-8') as fh:
        body = fh.read()
    check('version:' in body and '__CIV_VERSION__' not in body,
          'skill install: metadata.version rendered (no placeholder)')
    check(os.path.isdir(os.path.join(ISO_HOME, '.claude', 'skills', 'civ-prompt', 'references')),
          'skill install: references/ copied')

# skill update: same version -> up-to-date
su = json_of('skill_update')
su = su if isinstance(su, list) else []
check(exit_of('skill_update') == 0 and len(su) == 1
      and su[0].get('action') == 'up-to-date',
      'skill update: exit 0, up-to-date (version match)')

# skill remove: zcode directory deleted
sr = json_of('skill_remove')
sr = sr if isinstance(sr, list) else []
check(exit_of('skill_remove') == 0 and len(sr) == 1
      and sr[0].get('action') == 'removed' and not os.path.exists(zcode_skill),
      'skill remove: exit 0, zcode directory deleted')

# skill invalid platform -> exit 1
check(exit_of('skill_bad') == 1,
      'skill install --platform bogus: exit 1 (usage)')

# skill interactive fallback (task 11-22-skill-interactive-menu): piped
# stdin drives the text prompts with one fewer step — no Proceed
# confirmation — and the planted .claude marker preselects claude, whose
# empty answer accepts it.
claude_re = os.path.join(INT_HOME, '.claude', 'skills', 'civ-prompt', 'SKILL.md')
check(exit_of('skill_interactive') == 0
      and 'Select platforms' in err_of('skill_interactive')
      and 'Proceed?' not in err_of('skill_interactive'),
      'skill install (piped stdin): exit 0 via text fallback, no Proceed? confirmation')
check(exit_of('skill_interactive') == 0
      and 'installed' in out_of('skill_interactive')
      and os.path.isfile(claude_re),
      'skill install (piped stdin): preselected claude installed')

# upgrade --proxy to a refused port fails fast (exit 1) before the release
# check — the transport error surfaces immediately, no network wait (task
# 11-23-upgrade-direct).
check(exit_of('upgrade_proxy_dead') == 1,
      'upgrade --proxy <dead port>: exit 1 (fast failure, no network wait)')

# upgrade: graceful (no release -> exit 1; after release -> exit 0), no panic
check(exit_of('upgrade') in (0, 1) and (out_of('upgrade') or err_of('upgrade')),
      'upgrade: graceful exit 0/1 with a message (no panic)')

print()
if failures:
    sys.exit('[integration] %d check(s) FAILED' % len(failures))
print('[integration] all checks passed')
PYEOF