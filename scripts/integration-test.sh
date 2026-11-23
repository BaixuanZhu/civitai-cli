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
run search_json      search anima --json --limit 3
run search_table     search stocking --type lora --limit 5
run search_filters   search girl --base Illustrious --sort most-downloaded --period all --type lora --json --limit 3
run show_json        show 2458426 --json
run show_version     show 2458426 -v 3263843 --json
run show_prompts     show 2458426 -v 3263843 --prompts --json
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
for c in search show vet sync tags login logout proxy skill upgrade; do
  run "help_${c}_kw"   "$c" help
  run "help_${c}_flag" "$c" -h
done
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
run upgrade          upgrade

echo "[integration] asserting responses..."
python - "$WORK" "$ISO_HOME" "$INT_HOME" <<'PYEOF'
import json
import os
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

# search --json: raw items array passthrough with core fields
items = json_of('search_json')
items = items if isinstance(items, list) else []
check(exit_of('search_json') == 0 and len(items) > 0
      and all('id' in i and 'name' in i and 'type' in i for i in items[:3]),
      'search --json: non-empty items array carrying id/name/type')

# search table: human header present, stdout non-empty
check(exit_of('search_table') == 0 and 'NAME' in out_of('search_table'),
      'search table: exit 0 and header row present')

# search filters: --base/--sort/--period/--type accepted, valid JSON array
filt = json_of('search_filters')
check(exit_of('search_filters') == 0 and isinstance(filt, list)
      and (len(filt) == 0 or all('id' in i and 'name' in i for i in filt)),
      'search filters (--base/--sort/--period/--type): exit 0 and JSON array')

# show --json: structured model object
model = json_of('show_json')
check(exit_of('show_json') == 0 and isinstance(model, dict)
      and ('id' in model or 'name' in model),
      'show --json: exit 0 and model object with id/name')

# show version drill --json: parses (version detail object)
ver = json_of('show_version')
check(exit_of('show_version') == 0 and ver is not None,
      'show -v --json: exit 0 and valid JSON version detail')

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

# vet AutoV2 hit
check(exit_of('vet_autov2') == 0,
      'vet AutoV2: exit 0 (same version hit)')

# vet --prompts --json: recipe view
vp = json_of('vet_prompts')
check(exit_of('vet_prompts') == 0 and isinstance(vp, dict)
      and isinstance(vp.get('prompts'), list) and len(vp['prompts']) > 0,
      'vet --prompts --json: non-empty recipes')

# vet unknown hash -> exit 3 (not registered)
check(exit_of('vet_miss') == 3,
      'vet unknown hash: exit 3 (not registered)')

# vet bad input (8 hex -> path semantics, missing file) -> exit 1
check(exit_of('vet_badinput') == 1,
      'vet bad input: exit 1 (usage-class)')

# sync: not_found data rows, overall exit 0
check(exit_of('sync') == 0 and 'not_found' in out_of('sync'),
      'sync synthetic dir: exit 0 with not_found rows')

# sync --json: array of not_found rows
sj = json_of('sync_json')
check(exit_of('sync_json') == 0 and isinstance(sj, list) and len(sj) > 0
      and all(r.get('status') == 'not_found' for r in sj),
      'sync --json: array of not_found rows')

# usage error: limit over 100 rejected CLI-side -> exit 1
check(exit_of('usage_error') == 1,
      'search --limit 101: exit 1 (CLI-side cap)')

# help / unknown command: usage surface (offline)
check(exit_of('help_cmd') == 0 and 'Usage:' in out_of('help_cmd')
      and 'proxy' in out_of('help_cmd'),
      'civ help: exit 0, usage printed, proxy listed')
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
    'vet': [['Exit codes:', '3']],
    'sync': [['--concurrency']],
    'tags': [['--limit']],
    'login': [['--key']],
    'logout': [['logout']],
    'proxy': [['socks5']],
    'skill': [['--platform']],
    'upgrade': [['upgrade']],
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

# upgrade: graceful (no release -> exit 1; after release -> exit 0), no panic
check(exit_of('upgrade') in (0, 1) and (out_of('upgrade') or err_of('upgrade')),
      'upgrade: graceful exit 0/1 with a message (no panic)')

print()
if failures:
    sys.exit('[integration] %d check(s) FAILED' % len(failures))
print('[integration] all checks passed')
PYEOF