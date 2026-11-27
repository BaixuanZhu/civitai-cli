package cmd

// This file holds the four static shell completion scripts printed by
// `civ completion <shell>`. They are hand-written against completionSpec in
// completion.go and guarded by the drift-guard tests in completion_test.go
// (prd R3/R5): static text keeps shell startup cost at zero and needs no
// `civ` subprocess at completion time. When a flag or command is added,
// update completionSpec, these scripts, and the tests together.
//
// Shared invariants across all four scripts:
//   - position 1 completes command names only; flags never complete there
//     (the CLI accepts global flags only after the command name);
//   - every user-facing command word completes: search show vet sync tags
//     login logout proxy skill upgrade completion version help — never
//     unbootstrap (design D7);
//   - after the command word, per-command flags plus the five global flags
//     complete; enumerable flags also complete their values;
//   - free-form values (--base, --tag, --user, --limit, ids, keys, URLs,
//     --token, --proxy, --timeout) get no value candidates;
//   - `civ proxy set` completes the http:// https:// socks5:// socks5h://
//     scheme prefixes.

// bashCompletionScript is the bash completion program for civ. It completes
// command names in position 1, enum values after the flag (space and
// --flag=value forms), action words for proxy/skill/completion, and
// per-command flags plus the globals after the command word. Uses only
// compgen -W, so it works on Git Bash (Windows) and other minimal bash
// builds.
const bashCompletionScript = `# bash completion for the civ CLI (static script, civ completion bash).
# Install for this session:  source <(civ completion bash)
# Install persistently:      civ completion bash > /etc/bash_completion.d/civ

_civ() {
    local cur prev cmd
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    cmd="${COMP_WORDS[1]}"

    # Command names complete only in position 1; flags never do.
    if [ "$COMP_CWORD" -eq 1 ]; then
        COMPREPLY=( $(compgen -W "search show recipe resolve compare vet sync tags login logout proxy skill upgrade completion version help" -- "$cur") )
        return 0
    fi

    # Scheme prefixes for "civ proxy set <scheme>://<host>:<port>".
    if [ "$cmd" = "proxy" ] && [ "$COMP_CWORD" -eq 3 ] && [ "${COMP_WORDS[2]}" = "set" ]; then
        COMPREPLY=( $(compgen -W "http:// https:// socks5:// socks5h://" -- "$cur") )
        return 0
    fi

    # Enum values after a flag and a space.
    case "$prev" in
        --sort)
            if [ "$cmd" = "recipe" ]; then
                COMPREPLY=( $(compgen -W "most-reactions newest" -- "$cur") )
            else
                COMPREPLY=( $(compgen -W "hot highest-rated most-downloaded most-liked most-discussed most-collected most-images newest oldest recently-added" -- "$cur") )
            fi
            return 0
            ;;
        --view)
            COMPREPLY=( $(compgen -W "compact full" -- "$cur") )
            return 0
            ;;
        --source)
            COMPREPLY=( $(compgen -W "official community" -- "$cur") )
            return 0
            ;;
        --period)
            COMPREPLY=( $(compgen -W "day week month year all" -- "$cur") )
            return 0
            ;;
        --type)
            COMPREPLY=( $(compgen -W "checkpoint lora locon ti textualinversion hypernet hypernetwork dora controlnet upscaler motionmodule vae textencoder unet clipvision poses wildcards workflows comfyworkflows detection visionlanguage clip llm aestheticgradient other" -- "$cur") )
            return 0
            ;;
        --format)
            COMPREPLY=( $(compgen -W "table csv json" -- "$cur") )
            return 0
            ;;
        --scope)
            COMPREPLY=( $(compgen -W "user project" -- "$cur") )
            return 0
            ;;
        --platform)
            COMPREPLY=( $(compgen -W "claude zcode codex all" -- "$cur") )
            return 0
            ;;
        # Free-form flag values (incl. the global value flags): no candidates.
        --base|--tag|--user|--limit|--examples|--example|-v|--ver|--ext|--concurrency|--key|-t|--token|--proxy|--timeout)
            return 0
            ;;
    esac

    # Enum values in --flag=value form.
    case "$cur" in
        --sort=*)     COMPREPLY=( $(compgen -W "hot highest-rated most-downloaded most-liked most-discussed most-collected most-images newest oldest recently-added" -- "${cur#--sort=}") ); return 0 ;;
        --period=*)   COMPREPLY=( $(compgen -W "day week month year all" -- "${cur#--period=}") ); return 0 ;;
        --type=*)     COMPREPLY=( $(compgen -W "checkpoint lora locon ti textualinversion hypernet hypernetwork dora controlnet upscaler motionmodule vae textencoder unet clipvision poses wildcards workflows comfyworkflows detection visionlanguage clip llm aestheticgradient other" -- "${cur#--type=}") ); return 0 ;;
        --format=*)   COMPREPLY=( $(compgen -W "table csv json" -- "${cur#--format=}") ); return 0 ;;
        --scope=*)    COMPREPLY=( $(compgen -W "user project" -- "${cur#--scope=}") ); return 0 ;;
        --platform=*) COMPREPLY=( $(compgen -W "claude zcode codex all" -- "${cur#--platform=}") ); return 0 ;;
        --view=*)     COMPREPLY=( $(compgen -W "compact full" -- "${cur#--view=}") ); return 0 ;;
        --source=*)   COMPREPLY=( $(compgen -W "official community" -- "${cur#--source=}") ); return 0 ;;
    esac

    # Action words for the action-style commands.
    if [ "$COMP_CWORD" -eq 2 ]; then
        case "$cmd" in
            proxy)      COMPREPLY=( $(compgen -W "set get unset" -- "$cur") ); return 0 ;;
            skill)      COMPREPLY=( $(compgen -W "install remove update" -- "$cur") ); return 0 ;;
            completion) COMPREPLY=( $(compgen -W "bash zsh fish powershell" -- "$cur") ); return 0 ;;
        esac
    fi

    # Only flags complete from here on; positional arguments never do.
    case "$cur" in
        -*) ;;
        *) return 0 ;;
    esac

    # Per-command flags plus the five global flags (unreachable in
    # position 1 by construction, matching the CLI's accepted position).
    case "$cmd" in
        search)     COMPREPLY=( $(compgen -W "--type --base --sort --period --tag --user --limit --view --json --token --proxy --timeout --verbose" -- "$cur") ) ;;
        show)       COMPREPLY=( $(compgen -W "-v --ver --prompts --view --json --token --proxy --timeout --verbose" -- "$cur") ) ;;
        recipe)     COMPREPLY=( $(compgen -W "--examples --example --source --sort --limit --stack-summary --json --token --proxy --timeout --verbose" -- "$cur") ) ;;
        resolve)    COMPREPLY=( $(compgen -W "--json --token --proxy --timeout --verbose" -- "$cur") ) ;;
        compare)    COMPREPLY=( $(compgen -W "--diff --json --token --proxy --timeout --verbose" -- "$cur") ) ;;
        vet)        COMPREPLY=( $(compgen -W "--prompts --json --token --proxy --timeout --verbose" -- "$cur") ) ;;
        sync)       COMPREPLY=( $(compgen -W "--ext --format --concurrency --rehash --json --token --proxy --timeout --verbose" -- "$cur") ) ;;
        tags)       COMPREPLY=( $(compgen -W "--limit --json --token --proxy --timeout --verbose" -- "$cur") ) ;;
        login)      COMPREPLY=( $(compgen -W "--key -t --json --token --proxy --timeout --verbose" -- "$cur") ) ;;
        logout)     COMPREPLY=( $(compgen -W "--json --token --proxy --timeout --verbose" -- "$cur") ) ;;
        proxy)      COMPREPLY=( $(compgen -W "--github --json --token --proxy --timeout --verbose" -- "$cur") ) ;;
        skill)      COMPREPLY=( $(compgen -W "--platform --scope --force --json --token --proxy --timeout --verbose" -- "$cur") ) ;;
        upgrade)    COMPREPLY=( $(compgen -W "--json --token --proxy --timeout --verbose" -- "$cur") ) ;;
        completion) COMPREPLY=( $(compgen -W "--json --token --proxy --timeout --verbose" -- "$cur") ) ;;
        version)    COMPREPLY=( $(compgen -W "--json --token --proxy --timeout --verbose" -- "$cur") ) ;;
        help)       COMPREPLY=( $(compgen -W "--json --token --proxy --timeout --verbose" -- "$cur") ) ;;
        *)          return 0 ;;
    esac
    return 0
}

complete -F _civ civ
`

// zshCompletionScript is the zsh completion program for civ: #compdef civ
// plus an _arguments -C dispatch — command words in position 1, then a
// per-command nested _arguments spec offering the command's flags, the five
// globals, and (enum flags) their value sets. Proxy/skill/completion
// positional arguments complete via _values on CURRENT == 3.
const zshCompletionScript = `#compdef civ
# zsh completion for the civ CLI (static script, civ completion zsh).
# Install: civ completion zsh > "${fpath[1]}/_civ"   (any directory in $fpath)

_civ() {
    local curcontext="$curcontext" state line
    typeset -A opt_args

    # The five global flags, offered only inside the args state — i.e.
    # only after the command word, the position the CLI accepts them in.
    local -a global_specs
    global_specs=(
        '--json[machine-readable output]'
        '--token[API token]:token:'
        '--proxy[proxy URL]:url:'
        '--timeout[per-request timeout]:seconds:'
        '--verbose[verbose logging]'
    )

    _arguments -C \
        '1:command:->cmds' \
        '*:arguments:->args'

    case "$state" in
        cmds)
            local -a commands
            commands=(
                'search:search Civitai models'
                'show:show a model header and version table'
                'recipe:show compact official/community recipe context'
                'resolve:evaluate a resource set local readiness'
                'compare:compare model or version facts'
                'vet:audit a local file or hash against the registry'
                'sync:batch-audit a directory of model files'
                'tags:tag-name completion for prompt engineering'
                'login:save an API token to the config file'
                'logout:remove the saved API token'
                'proxy:save, show, or clear the persisted proxy'
                'skill:install, remove, or update the bundled skill'
                'upgrade:self-update from the latest release'
                'completion:print a shell completion script'
                'version:print the civ version'
                'help:print the command overview'
            )
            _describe -t commands 'civ command' commands
            ;;
        args)
            case "$line[1]" in
                search)
                    _arguments \
                        '--type[model type filter]:model type:(checkpoint lora locon ti textualinversion hypernet hypernetwork dora controlnet upscaler motionmodule vae textencoder unet clipvision poses wildcards workflows comfyworkflows detection visionlanguage clip llm aestheticgradient other)' \
                        '--base[base model filter]:base models:' \
                        '--sort[sort order]:sort order:(hot highest-rated most-downloaded most-liked most-discussed most-collected most-images newest oldest recently-added)' \
                        '--period[ranking period]:period:(day week month year all)' \
                        '--tag[exact tag filter]:tag:' \
                        '--user[creator username]:user:' \
                        '--limit[max results]:count:' \
                        '--view[json detail level]:view:(compact full)' \
                        "${global_specs[@]}"
                    ;;
                show)
                    _arguments \
                        '(--ver)-v[drill into this version id]:version id:' \
                        '(-v)--ver[alias of -v]:version id:' \
                        '--prompts[append example-image recipes]' \
                        '--view[json detail level]:view:(compact full)' \
                        "${global_specs[@]}"
                    ;;
                recipe)
                    _arguments \
                        '--examples[summary example count]:count:' \
                        '--example[selected example detail index]:source index:' \
                        '--source[recipe source]:source:(official community)' \
                        '--sort[community sample order]:order:(most-reactions newest)' \
                        '--limit[community sample limit]:count:' \
                        '--stack-summary[aggregate-only community stack summary]' \
                        "${global_specs[@]}"
                    ;;
                compare)
                    _arguments \
                        '--diff[hide confirmed-equal fields]' \
                        "${global_specs[@]}"
                    ;;
                vet)
                    _arguments \
                        '--prompts[append example-image recipes]' \
                        "${global_specs[@]}"
                    ;;
                sync)
                    _arguments \
                        '--ext[extensions to scan]:extensions:' \
                        '--format[output format]:format:(table csv json)' \
                        '--concurrency[API concurrency]:count:' \
                        '--rehash[recompute every hash]' \
                        "${global_specs[@]}"
                    ;;
                tags)
                    _arguments \
                        '--limit[max tag names]:count:' \
                        "${global_specs[@]}"
                    ;;
                login)
                    _arguments \
                        '(-t)--key[API key to save, - reads stdin]:token:' \
                        '(--key)-t[shorthand for --key]:token:' \
                        "${global_specs[@]}"
                    ;;
                proxy)
                    if (( CURRENT == 3 )); then
                        _values 'proxy action' \
                            'set[save a proxy URL]' \
                            'get[show the proxy status]' \
                            'unset[clear the saved proxy]'
                    elif (( CURRENT == 4 )) && [[ "${words[3]}" == set ]]; then
                        _values 'proxy scheme' \
                            'http://[plain HTTP proxy]' \
                            'https://[TLS-terminating HTTP proxy]' \
                            'socks5://[SOCKS5 proxy]' \
                            'socks5h://[SOCKS5 proxy, remote DNS]'
                    else
                        _arguments \
                            '--github[target the GitHub-only proxy slot (set/unset)]' \
                            "${global_specs[@]}"
                    fi
                    ;;
                skill)
                    if (( CURRENT == 3 )); then
                        _values 'skill action' \
                            'install[install the civ-prompt skill]' \
                            'remove[remove the civ-prompt skill]' \
                            'update[update the civ-prompt skill]'
                    else
                        _arguments \
                            '--platform[target platforms]:platforms:(claude zcode codex all)' \
                            '--scope[install scope]:scope:(user project)' \
                            '--force[overwrite without prompting]' \
                            "${global_specs[@]}"
                    fi
                    ;;
                completion)
                    if (( CURRENT == 3 )); then
                        _values 'shell' \
                            'bash[bash completion script]' \
                            'zsh[zsh completion script]' \
                            'fish[fish completion script]' \
                            'powershell[PowerShell completion script]'
                    else
                        _arguments "${global_specs[@]}"
                    fi
                    ;;
                resolve|logout|upgrade|version|help)
                    _arguments "${global_specs[@]}"
                    ;;
            esac
            ;;
    esac
}

_civ "$@"
`

// fishCompletionScript is the fish completion program for civ. Command
// names use __fish_use_subcommand (position 1 only); every per-command or
// global candidate requires __fish_seen_subcommand_from, so flags complete
// only after the command word. -x marks value flags as "requires a
// parameter, no file completion"; enum candidates ride on -a. (Known
// limitation: the proxy-set scheme list re-offers after the URL token; it
// is a display-only quirk of the static script.)
const fishCompletionScript = `# fish completion for the civ CLI (static script, civ completion fish).
# Install for this session:  civ completion fish | source
# Install persistently:      civ completion fish > ~/.config/fish/completions/civ.fish

# Command names only while no subcommand is present yet (position 1).
complete -c civ -n '__fish_use_subcommand' -a 'search show recipe resolve compare vet sync tags login logout proxy skill upgrade completion version help'

# civ search
complete -c civ -n '__fish_seen_subcommand_from search' -l type -x -a 'checkpoint lora locon ti textualinversion hypernet hypernetwork dora controlnet upscaler motionmodule vae textencoder unet clipvision poses wildcards workflows comfyworkflows detection visionlanguage clip llm aestheticgradient other'
complete -c civ -n '__fish_seen_subcommand_from search' -l base -x
complete -c civ -n '__fish_seen_subcommand_from search' -l sort -x -a 'hot highest-rated most-downloaded most-liked most-discussed most-collected most-images newest oldest recently-added'
complete -c civ -n '__fish_seen_subcommand_from search' -l period -x -a 'day week month year all'
complete -c civ -n '__fish_seen_subcommand_from search' -l tag -x
complete -c civ -n '__fish_seen_subcommand_from search' -l user -x
complete -c civ -n '__fish_seen_subcommand_from search' -l limit -x
complete -c civ -n '__fish_seen_subcommand_from search' -l view -x -a 'compact full'

# civ show
complete -c civ -n '__fish_seen_subcommand_from show' -s v -l ver -x -d 'Drill into this version id'
complete -c civ -n '__fish_seen_subcommand_from show' -l prompts -d 'Append example-image recipes'
complete -c civ -n '__fish_seen_subcommand_from show' -l view -x -a 'compact full'

# civ recipe
complete -c civ -n '__fish_seen_subcommand_from recipe' -l examples -x -d 'Summary example count (0-10)'
complete -c civ -n '__fish_seen_subcommand_from recipe' -l example -x -d 'Selected example detail index'
complete -c civ -n '__fish_seen_subcommand_from recipe' -l source -x -a 'official community'
complete -c civ -n '__fish_seen_subcommand_from recipe' -l sort -x -a 'most-reactions newest'
complete -c civ -n '__fish_seen_subcommand_from recipe' -l limit -x -d 'Community sample limit (1-100)'
complete -c civ -n '__fish_seen_subcommand_from recipe' -l stack-summary -d 'Aggregate-only community stack summary'

# civ compare
complete -c civ -n '__fish_seen_subcommand_from compare' -l diff -d 'Hide confirmed-equal fields'

# civ vet
complete -c civ -n '__fish_seen_subcommand_from vet' -l prompts -d 'Append example-image recipes'

# civ sync
complete -c civ -n '__fish_seen_subcommand_from sync' -l ext -x -d 'Extensions to scan'
complete -c civ -n '__fish_seen_subcommand_from sync' -l format -x -a 'table csv json'
complete -c civ -n '__fish_seen_subcommand_from sync' -l concurrency -x -d 'API concurrency (1-8)'
complete -c civ -n '__fish_seen_subcommand_from sync' -l rehash -d 'Recompute every hash'

# civ tags
complete -c civ -n '__fish_seen_subcommand_from tags' -l limit -x -d 'Max tag names (1-100)'

# civ login
complete -c civ -n '__fish_seen_subcommand_from login' -l key -x -d 'API key to save ("-" reads stdin)'
complete -c civ -n '__fish_seen_subcommand_from login' -s t -x -d 'Shorthand for --key'

# civ proxy — actions first, then schemes for "proxy set", then flags
complete -c civ -n '__fish_seen_subcommand_from proxy' -n 'not __fish_seen_subcommand_from set get unset' -a 'set get unset'
complete -c civ -n '__fish_seen_subcommand_from proxy' -n '__fish_seen_subcommand_from set' -a 'http:// https:// socks5:// socks5h://'
complete -c civ -n '__fish_seen_subcommand_from proxy' -l github -d 'Target the GitHub-only proxy slot (set/unset)'

# civ skill — actions first, then flags
complete -c civ -n '__fish_seen_subcommand_from skill' -n 'not __fish_seen_subcommand_from install remove update' -a 'install remove update'
complete -c civ -n '__fish_seen_subcommand_from skill' -l platform -x -a 'claude zcode codex all'
complete -c civ -n '__fish_seen_subcommand_from skill' -l scope -x -a 'user project'
complete -c civ -n '__fish_seen_subcommand_from skill' -l force -d 'Overwrite without prompting'

# civ completion — shell names as the first argument
complete -c civ -n '__fish_seen_subcommand_from completion' -n 'not __fish_seen_subcommand_from bash zsh fish powershell' -a 'bash zsh fish powershell'

# Global flags: only after a command word, matching the CLI's parsing.
# --token/--proxy/--timeout take free-form values (-x offers no candidates);
# --json/--verbose are switches.
set -l civ_global_condition '__fish_seen_subcommand_from search show recipe resolve compare vet sync tags login logout proxy skill upgrade completion version help'
complete -c civ -n "$civ_global_condition" -l json -d 'Machine-readable output'
complete -c civ -n "$civ_global_condition" -l token -x -d 'API token'
complete -c civ -n "$civ_global_condition" -l proxy -x -d 'Proxy URL for this invocation'
complete -c civ -n "$civ_global_condition" -l timeout -x -d 'Per-request timeout'
complete -c civ -n "$civ_global_condition" -l verbose -d 'Verbose logging'
`

// powershellCompletionScript is the native argument completer for civ,
// registered for Windows PowerShell 5.1+ (no ternary operator, no ??
// operator, no class syntax). It mirrors the other three scripts: command
// words in position 1, enum values after enumerable flags (space and
// --flag=value forms), action words for proxy/skill/completion, proxy-set
// scheme prefixes, then per-command flags plus the five globals — the
// globals only after the command word, by construction.
const powershellCompletionScript = `# powershell completion for the civ CLI (static script, civ completion powershell).
# Install for this session:  civ completion powershell | Out-String | Invoke-Expression
# Install persistently:      add that line to $PROFILE
# Windows PowerShell 5.1 compatible: no ternary operator, no ?? operator.

Register-ArgumentCompleter -Native -CommandName 'civ' -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    $commands = @('search', 'show', 'recipe', 'resolve', 'compare', 'vet', 'sync', 'tags', 'login', 'logout', 'proxy', 'skill', 'upgrade', 'completion', 'version', 'help')
    $globalFlags = @('--json', '--token', '--proxy', '--timeout', '--verbose')

    # Enumerable flags: flag name -> value candidates.
    $enumValues = @{
        '--sort'     = @('hot', 'highest-rated', 'most-downloaded', 'most-liked', 'most-discussed', 'most-collected', 'most-images', 'newest', 'oldest', 'recently-added')
        '--period'   = @('day', 'week', 'month', 'year', 'all')
        '--type'     = @('checkpoint', 'lora', 'locon', 'ti', 'textualinversion', 'hypernet', 'hypernetwork', 'dora', 'controlnet', 'upscaler', 'motionmodule', 'vae', 'textencoder', 'unet', 'clipvision', 'poses', 'wildcards', 'workflows', 'comfyworkflows', 'detection', 'visionlanguage', 'clip', 'llm', 'aestheticgradient', 'other')
        '--format'   = @('table', 'csv', 'json')
        '--scope'    = @('user', 'project')
        '--platform' = @('claude', 'zcode', 'codex', 'all')
        '--view'     = @('compact', 'full')
        '--source'   = @('official', 'community')
    }

    # Flags taking a free-form value (including the global value flags):
    # never completed.
    $valueFlags = @('--base', '--tag', '--user', '--limit', '--examples', '--example', '-v', '--ver', '--ext', '--concurrency', '--key', '-t', '--token', '--proxy', '--timeout')

    # Per-command flags; the globals are appended after the command word.
    $commandFlags = @{
        'search'     = @('--type', '--base', '--sort', '--period', '--tag', '--user', '--limit', '--view')
        'show'       = @('-v', '--ver', '--prompts', '--view')
        'recipe'     = @('--examples', '--example', '--source', '--sort', '--limit', '--stack-summary')
        'resolve'    = @()
        'compare'    = @('--diff')
        'vet'        = @('--prompts')
        'sync'       = @('--ext', '--format', '--concurrency', '--rehash')
        'tags'       = @('--limit')
        'login'      = @('--key', '-t')
        'logout'     = @()
        'proxy'      = @('--github')
        'skill'      = @('--platform', '--scope', '--force')
        'upgrade'    = @()
        'completion' = @()
        'version'    = @()
        'help'       = @()
    }

    $results = @()

    # Tokenize the line, dropping the word currently being completed (the
    # last element when it equals $wordToComplete).
    $texts = @()
    foreach ($element in $commandAst.CommandElements) {
        $texts += $element.Extent.Text
    }
    $tokens = @()
    for ($i = 0; $i -lt $texts.Count; $i++) {
        if ($i -eq $texts.Count - 1 -and $texts[$i] -eq $wordToComplete) {
            continue
        }
        $tokens += $texts[$i]
    }

    # Position 1 completes command names only; flags never do.
    if ($tokens.Count -le 1) {
        foreach ($c in $commands) {
            if ($c -like "$wordToComplete*") {
                $results += [System.Management.Automation.CompletionResult]::new($c, $c, 'ParameterValue', $c)
            }
        }
        return $results
    }

    $cmd = $tokens[1]
    $prev = ''
    if ($tokens.Count -ge 2) {
        $prev = $tokens[$tokens.Count - 1]
    }
    $afterCmd = $tokens.Count - 2

    # Recipe's community sort uses its own vocabulary; handle it before the
    # shared search-sort candidates.
    if ($cmd -eq 'recipe' -and $prev -eq '--sort') {
        foreach ($v in @('most-reactions', 'newest')) {
            if ($v -like "$wordToComplete*") {
                $results += [System.Management.Automation.CompletionResult]::new($v, $v, 'ParameterValue', $v)
            }
        }
        return $results
    }

    # Enum values after "--flag " (space form).
    if ($enumValues.ContainsKey($prev)) {
        foreach ($v in $enumValues[$prev]) {
            if ($v -like "$wordToComplete*") {
                $results += [System.Management.Automation.CompletionResult]::new($v, $v, 'ParameterValue', $v)
            }
        }
        return $results
    }

    # Free-form flag values: no candidates.
    if ($valueFlags -contains $prev) {
        return $results
    }

    # Enum values in --flag=value form.
    foreach ($flag in $enumValues.Keys) {
        if ($wordToComplete -like "$flag=*") {
            $prefix = $flag + '='
            $partial = $wordToComplete.Substring($prefix.Length)
            foreach ($v in $enumValues[$flag]) {
                if ($v -like "$partial*") {
                    $value = $prefix + $v
                    $results += [System.Management.Automation.CompletionResult]::new($value, $value, 'ParameterValue', $value)
                }
            }
            return $results
        }
    }

    # Action words for the action-style commands (first argument only).
    if ($afterCmd -eq 0) {
        $actions = @()
        if ($cmd -eq 'proxy') { $actions = @('set', 'get', 'unset') }
        elseif ($cmd -eq 'skill') { $actions = @('install', 'remove', 'update') }
        elseif ($cmd -eq 'completion') { $actions = @('bash', 'zsh', 'fish', 'powershell') }
        if ($actions.Count -gt 0) {
            foreach ($a in $actions) {
                if ($a -like "$wordToComplete*") {
                    $results += [System.Management.Automation.CompletionResult]::new($a, $a, 'ParameterValue', $a)
                }
            }
            return $results
        }
    }

    # "civ proxy set <scheme>://<host>:<port>"
    if ($cmd -eq 'proxy' -and $afterCmd -eq 1 -and $tokens[2] -eq 'set') {
        foreach ($s in @('http://', 'https://', 'socks5://', 'socks5h://')) {
            if ($s -like "$wordToComplete*") {
                $results += [System.Management.Automation.CompletionResult]::new($s, $s, 'ParameterValue', $s)
            }
        }
        return $results
    }

    # Per-command flags plus the five globals, only after the command word
    # (this branch is unreachable in position 1 by construction).
    $flags = @()
    if ($commandFlags.ContainsKey($cmd)) {
        $flags = $commandFlags[$cmd] + $globalFlags
    } else {
        return $results
    }
    foreach ($f in $flags) {
        if ($f -like "$wordToComplete*") {
            $results += [System.Management.Automation.CompletionResult]::new($f, $f, 'ParameterValue', $f)
        }
    }
    return $results
}
`
