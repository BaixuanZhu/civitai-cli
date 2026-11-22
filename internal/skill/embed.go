package skill

import "embed"

// SkillName is the bundled skill's directory name and frontmatter name. The
// agentskills.io standard requires name to equal the parent directory name
// (kebab-case), so install always writes to <root>/civ-prompt/.
const SkillName = "civ-prompt"

// embedRoot is the go:embed path prefix of the skill tree inside FS.
const embedRoot = "assets/" + SkillName

// FS embeds the civ-prompt skill tree (the SKILL.md template plus references/)
// into every built civ binary. This makes the skill self-carrying: any installed
// civ can run "civ skill install" without the repository source on disk
// (design §3, prd R15/R16). The SKILL.md template carries a __CIV_VERSION__
// placeholder rendered to app.Version at install/update time.
//
//go:embed assets/civ-prompt
var FS embed.FS
