# Research: 三平台 Skill 目录约定与 SKILL.md 格式（zcode / Claude Code / codex）

- **Query**: civ skill install/remove/update 需要准确的三平台 skill 目录约定、SKILL.md 格式、发现机制、平台差异，用于设计文档
- **Scope**: mixed（本机实证 + 官方文档）
- **Date**: 2026-11-22

## TL;DR（结论先行）

1. **三家都遵循同一个开放标准 [agentskills.io](https://agentskills.io) 的 `SKILL.md` 规范** → 一份可移植的 `SKILL.md`（仅用标准六字段）可同时服务三平台，**frontmatter 无需平台差异化**。
2. **codex 在 2026 年已原生支持 skills**（不再是「无原生 skill、只能用 AGENTS.md」）。官方文档明确：codex 从 **`.agents/skills/`** 读取，**不是 `.codex/skills/`**。
3. **三平台原生 skill 根目录不同**，但 **`.agents/skills/` 是 zcode + codex 共享根**；Claude Code 不原生读 `.agents/skills/`，需 `.claude/skills/` 副本或符号链接桥接。
4. `name` 字段在三家都**必须等于目录名**（kebab-case，1–64 字符）。

---

## 三平台对照表

| 维度 | Claude Code | zcode | codex (OpenAI Codex CLI) |
|---|---|---|---|
| **用户级全局目录** | `~/.claude/skills/<name>/SKILL.md` | `~/.zcode/skills/<name>/`（优先）+ `~/.agents/skills/<name>/`（共享，次优先） | `$HOME/.agents/skills/<name>/`（**不是** `~/.codex/skills/`） |
| **项目级目录** | `.claude/skills/<name>/SKILL.md`（从 CWD 向上扫到 repo root） | `.zcode/skills/<name>/`（优先）+ `.agents/skills/<name>/`（共享） | `$CWD/.agents/skills`，向上每层目录直到 `$REPO_ROOT/.agents/skills` |
| **其它来源** | Enterprise（managed settings 目录）、`--add-dir`、plugin skills、claude.ai synced（`~/.claude/skills/synced/`） | plugin marketplace（镜像 `claude-plugins-official` + 自有 `zcode-plugins-official`）、skillhub.cn store | ADMIN `/etc/codex/skills`；SYSTEM 内置（`skill-creator`、`skill-installer`）；plugins 分发 |
| **优先级 / 同名处理** | Enterprise > Personal > Project；同名 personal 覆盖 project | `.zcode/skills` > `.agents/skills`（项目级 > 用户级）；同名按路径区分，path 即安装身份 | 同名**不合并**，两个都出现在选择器里 |
| **SKILL.md 必填** | `description`（推荐）；`name` 可选，默认=目录名 | `name` + `description` | `name` + `description` |
| **标准可移植字段** | name, description, license, compatibility, metadata, allowed-tools | 同左（标准六字段） | 同左（agentskills.io 标准） |
| **平台扩展字段** | when_to_use, argument-hint, arguments, disable-model-invocation, user-invocable, disallowed-tools, model, effort, context, agent, background, hooks, paths, shell | 运行时容忍 Claude 风格扩展（本机实测 `disable-model-invocation`、`hidden`、`metadata.argument-hint`） | `agents/openai.yaml`（外观+依赖，可选支持文件）；`~/.codex/config.toml` 的 `[[skills.config]]` 控制启用 |
| **supporting files** | `scripts/` `references/` `assets/` + 任意文件 | `references/` `scripts/` `assets/`（实测也有扁平 `.md` + `agents/` 子目录） | `scripts/` `references/` `assets/` `agents/openai.yaml` |
| **符号链接** | 支持（读 target，多指向同一 target 只加载一次） | 支持（本机 `~/.zcode/skills/agent-browser` → `~/.agents/skills/agent-browser`） | 支持（扫描时跟随 symlink target） |
| **显式调用** | `/skill-name`（目录名或 name 字段） | `/skill <name> <prompt>` 强制加载；description 自动触发 | `/skills` 或 `$` 提及（CLI/IDE）；`@`（ChatGPT）；description 自动触发 |
| **热更新 / 移除** | 监听目录，无需重启；移除=删目录 | 自动发现 | 自动检测变更，未出现则重启；`[[skills.config]] enabled=false` 可禁用不删 |
| **渐进式加载预算** | metadata 常驻；body 触发时加载；description+when_to_use 列表截断 1536 字符 | 三层：metadata 常驻 / body <500 行 / 支持文件按需 | 初始列表=name+description+**文件路径**；预算 2% 上下文或 8000 字符；`skills.max_context_tokens` 上限 10000 tokens |

---

## SKILL.md 规范（agentskills.io 开放标准 —— 三家共同基线）

证据：<https://agentskills.io/specification.md>（官方）+ 本机 Anthropic `quick_validate.py`（见下）。

### 目录结构
```
skill-name/
├── SKILL.md          # 必需：YAML frontmatter + Markdown 正文
├── scripts/          # 可选：可执行代码
├── references/       # 可选：按需加载的文档
├── assets/           # 可选：模板/资源
└── ...               # 任意其它文件/目录
```

### Frontmatter 字段
| 字段 | 必填 | 约束 |
|---|---|---|
| `name` | 是 | 1–64 字符；仅小写 `a-z0-9` + 连字符；不能以 `-` 开头/结尾；不能有连续 `--`；**必须等于父目录名** |
| `description` | 是 | 1–1024 字符；非空；说明「做什么 + 何时用」，含触发关键词 |
| `license` | 否 | 许可证名或 bundled 许可文件引用（建议短） |
| `compatibility` | 否 | 1–500 字符；环境要求（目标产品/系统包/网络等）；多数 skill 不需要 |
| `metadata` | 否 | string→string 映射；客户端自定义属性；键名建议唯一 |
| `allowed-tools` | 否 | 空格分隔的预批准工具串，如 `Bash(git:*) Bash(jq:*) Read`；**实验性**，各实现支持度不一 |

- 正文：Markdown，无格式限制；推荐分步指令 / 输入输出示例 / 边界情况；过长拆到 `references/`。
- 渐进式披露三层：① metadata（name+description）常驻上下文；② SKILL.md body 激活时加载（建议 <500 行）；③ bundled 文件按需读取（脚本可不加载直接执行）。

### 本机权威校验器（Anthropic skill-creator）
`~/.claude/plugins/marketplaces/claude-plugins-official/plugins/skill-creator/skills/skill-creator/scripts/quick_validate.py:42`
```python
ALLOWED_PROPERTIES = {'name', 'description', 'license', 'allowed-tools', 'metadata', 'compatibility'}
```
- `name`：`^[a-z0-9-]+$`，禁首尾/连续连字符，≤64（line 65–71）
- `description`：禁尖括号 `< >`，≤1024（line 80–84）
- `compatibility`：字符串 ≤500（line 87–92）
- **注意**：`disable-model-invocation`、`hidden`、`argument-hint` 等**不在**此校验器允许集——它们是平台运行时扩展，不是可移植标准字段。打包/上传（claude.ai、Skills API、`package_skill.py`）遇到非标准字段会**硬报错**。

---

## 逐平台证据

### Claude Code（官方文档实证）
来源：<https://code.claude.com/docs/en/skills.md>
- 「Claude Code skills follow the [Agent Skills](https://agentskills.io) open standard」+ 扩展（invocation control / subagent execution / dynamic context injection）。
- 位置表（line 162–166）：Enterprise / Personal `~/.claude/skills/` / Project `.claude/skills/` / Nested `<subdir>/.claude/skills/` / Additional `--add-dir`。
- 优先级（line 215）：Enterprise > Personal > Project。
- 项目级从 CWD 向上扫到 repo root（line 181）。
- 符号链接支持（line 172）；保留名 `synced`（line 173）；热更新监听（line 322）；移除=删目录（line 334）。
- frontmatter 参考（line 401–422）：所有字段可选，仅 `description` 推荐；`name` 默认目录名；`metadata` 自由 map（Claude 不解析）；`license`/`compatibility` 属标准但 Claude 不 acted on。
- **可移植六字段**（line 431）：`name, description, license, compatibility, metadata, allowed-tools`；非标准字段打包硬报错（line 435–439）。
- 命令名来源（line 449–457）：目录名或 `name` 字段 → `/skill-name`。
- **关键差异**：位置表**不含 `.agents/skills/`** → Claude Code 不原生读共享根；本机用 symlink 桥接（`~/.claude/skills/agent-browser` → `~/.agents/skills/agent-browser`）。
- 本机实证：`~/.claude/skills/` 存在（仅 agent-browser symlink）。

### zcode（本机官方 skill-creator 实证）
来源：`~/.zcode/cli/plugins/cache/zcode-plugins-official/skill-creator/0.1.0/skills/skill-creator/SKILL.md`
- 发现目录（line 41–48，优先级从高到低）：
  1. `<project>/.zcode/skills/<name>/SKILL.md`
  2. `<project>/.agents/skills/<name>/SKILL.md`
  3. `~/.zcode/skills/<name>/SKILL.md`
  4. `~/.agents/skills/<name>/SKILL.md`
- 「Default to creating new skills under `.agents/skills/`」——跨工具标准位置；`.zcode/skills` 优先，是**override** 位置（line 48）。
- `name` kebab-case 1–64，**必须匹配目录名**（line 65）；`description` 主触发信号（line 66）。
- 三层渐进加载（line 72–76）；支持 `references/ scripts/ assets/`（line 54–61）。
- `/skill <name> <prompt>` 强制加载（line 110）；同名按路径区分，path=安装身份（line 146）。
- 更新保留 name+目录名；只读则复制到 `~/.agents/skills/<name>/`（line 144–145）。
- 本机 frontmatter 实例：
  - `.zcode/skills/trellis-check/SKILL.md:1-4` → 仅 `name`+`description`
  - `~/.zcode/skills/grill-me/SKILL.md:1-5` → `disable-model-invocation: true`
  - `~/.zcode/skills/web-design-guidelines/SKILL.md` → `metadata: {author: vercel, version: "1.0.0", argument-hint: <file-or-pattern>}`
  - `~/.agents/skills/agent-browser/SKILL.md:1-6` → `allowed-tools: Bash(agent-browser:*), Bash(npx agent-browser:*)` + `hidden: true`
  - supporting files 实例：`~/.zcode/skills/codebase-design/`（`DEEPENING.md`、`DESIGN-IT-TWICE.md`、`agents/`，SKILL.md 用相对 markdown 链接引用）
- zcode skill store：`~/.zcode/skills/.skills_store_lock.json` 记录 `{name, zip_url, source, version, installedAt}`（skillhub.cn）——zcode 自有版本跟踪，与 civ 无关。
- Trellis 部署 zcode skill 到 `.zcode/skills/`（`.qoder/skills/trellis-meta/references/platform-files/platform-map.md:26`、`local-architecture/bundled-skills.md:56`）。

### codex（官方文档实证 —— 初始不确定性已解决）
来源：<https://developers.openai.com/codex/skills.md>（repo `openai/codex/docs/skills.md` 指向此页）
- 「Use agent skills to extend ChatGPT and Codex… Skills build on the [open agent skills standard](https://agentskills.io)」→ **原生支持**。
- 加载位置表（官方）：
  | Scope | Location |
  |---|---|
  | REPO | `$CWD/.agents/skills`（向上每层到 `$REPO_ROOT/.agents/skills`） |
  | USER | `$HOME/.agents/skills` |
  | ADMIN | `/etc/codex/skills` |
  | SYSTEM | 内置 `skill-creator`、`skill-installer` |
- **明确不是 `.codex/skills/`**：官方表无此项。Trellis 创建「intentionally empty `.codex/skills/`」（`bundled-skills.md:66`），真实 skill 写 `.agents/skills/`（`bundled-skills.md:43`、`platform-map.md:12`）。
- SKILL.md 必需 `name`+`description`；可选 `scripts/ references/ assets/ agents/openai.yaml`。
- 同名不合并，都出现在选择器；支持 symlink；自动检测变更（未出现则重启）。
- 调用：`/skills` 或 `$` 提及；隐式靠 description。
- 预算：初始列表 name+description+路径，2% 上下文或 8000 字符；`skills.max_context_tokens` ≤10000。
- 启用/禁用：`~/.codex/config.toml` 的 `[[skills.config]]`（`path` 或 `name` 选择器 + `enabled=false`，最后匹配生效）；`[skills.bundled] enabled=false`。
- 安装 curated：`$skill-installer <name>`；分发用 plugins（ChatGPT/Codex 共享 universal plugin directory）。
- 本机：codex **未安装**（`~/.codex/` 不存在，`codex` 不在 PATH）→ 无法本机实证运行，但官方文档充分。详见 `codex-skill-uncertainty.md`。

### 跨平台共享根 `.agents/skills/`（Trellis 实证）
来源：`.qoder/skills/trellis-meta/references/platform-files/platform-map.md:75-77`、`local-architecture/bundled-skills.md:62`
- 「Codex, Gemini CLI, Pi Agent, Kimi Code, dsh write the shared `.agents/skills/` layer… the upstream Agent Skills workspace alias」。
- 「ZCode keeps Trellis-managed skills under `.zcode/skills/`」。
- 本机 `~/.agents/skills/`（agent-browser、filex）被 symlink 进 `~/.claude/skills/` 与 `~/.zcode/skills/` → 印证 `.agents/skills/` 是跨工具共享根，Claude 靠 symlink 桥接。

---

## 对 civ skill install 的设计建议

### 1. 平台 → skill 根目录映射（建议固定）
| 平台 | 用户级（global） | 项目级（project） |
|---|---|---|
| claude | `~/.claude/skills/<name>/` | `<repo>/.claude/skills/<name>/` |
| zcode | `~/.zcode/skills/<name>/` | `<repo>/.zcode/skills/<name>/` |
| codex | `~/.agents/skills/<name>/` | `<repo>/.agents/skills/<name>/` |

- **codex 必须用 `.agents/skills/`，不要用 `.codex/skills/`**（后者非 codex 读取位置）。
- `<name>` 建议固定为 `civ-prompt`（或 `civitai-prompt`），kebab-case，等于目录名。

### 2. 默认装哪 / 平台检测
- 交互式多选平台；非交互 flag 如 `--platform claude,zcode,codex`（或 `--all`）、`--scope user|project`。
- 自动探测已安装平台：`~/.claude/`、`~/.zcode/`、`~/.codex/`（或 `~/.agents/`）目录存在性 + PATH 二进制。codex 未装时仍可写 `~/.agents/skills/`（装后即读），但应提示/确认。
- **默认 scope 建议 user-level global**（契合「随版本分发的个人工具」）；project 用 flag 显式开启。
- **zcode + codex 共享 `.agents/skills/` 的去重**：若同 scope 同时选 zcode+codex，`.agents/skills/<name>` 一份即可被两者读取；zcode 额外写 `.zcode/skills/<name>` 会获得更高优先级（override），但 zcode 会同时看到两份（同名按路径区分，`.zcode` 胜出）。建议：选 zcode → 写 `.zcode/skills`（原生高优先）；选 codex → 写 `.agents/skills`；二者都选时接受 zcode 看到两份（文档说明），或仅写共享 `.agents/skills` 一份覆盖两者（更省，但失去 zcode override 语义）。**推荐前者（各写原生根），行为最可预测。**

### 3. 目录创建与写入
- `mkdir -p <root>/<name>/` 整树；从 `go:embed` 写 `SKILL.md` + supporting files（`references/`、`scripts/`、`assets/`）。
- **不依赖符号链接**：Windows 创建 symlink 需管理员/开发者模式；civ 是跨平台 Go CLI，默认写**真实副本**到各平台根。可选：检测到 dev-mode/admin 时才用 `.agents/skills` 共享 + symlink 桥接 Claude，否则回退副本。

### 4. name 冲突处理
- 目标 `<root>/<name>/` 已存在：读其 `SKILL.md` 的 `metadata.version` 判断是否 civ 旧版。
- install 遇冲突 → 提示 overwrite / skip / backup（`.bak`）；非交互 `--force` 覆盖。
- update → 保留 name+目录名，用新 embed 覆盖（agentskills.io/zcode/claude 均要求更新保留原名）。
- remove → 删 `<root>/<name>/` 整目录（三平台均热移除）。**不要动 zcode 的 `.skills_store_lock.json`**（那是 zcode 自有 store，civ 独立管理自己的 skill）。

### 5. SKILL.md 是否需平台差异化
- **不需要差异化 frontmatter**。一份仅含标准六字段的可移植 `SKILL.md` 三家通吃：
  - 必填 `name`（=目录名）、`description`（含文生图触发关键词：文生图/提示词/prompt/Stable Diffusion/Midjourney/SDXL/Flux/负向提示词等 + 何时用）。
  - 建议加 `license`（仓库 LICENSE）、`metadata.version`（= civ 版本，供 update 比对）。
  - 可选 `allowed-tools: Bash(civ:*)`（实验性；Claude  honors，codex/zcode 可能忽略——安全）。
  - **避免** Claude-only 字段（`disable-model-invocation`、`when_to_use`、`argument-hint`）写进 bundled 可移植 skill：会破坏 agentskills.io 打包校验，且 codex 行为未知。
- 正文：触发词 + 指引 agent 运行 `civ --prompts` 配方驱动提示词创作；body <500 行，长配方文档放 `references/`。
- 平台专属增强（如 codex `agents/openai.yaml`、Claude `disable-model-invocation`）若需要，应作为**可选附加文件**而非污染主 SKILL.md。

### 6. 版本/更新跟踪
- 把 civ 版本嵌入 `metadata.version`；`civ skill update` 比对已装 `metadata.version` vs 二进制版本，决定是否需要重写。
- 三平台均自动检测文件变更（codex 偶尔需重启），civ 无需调用平台 API，纯文件写入即可。

---

## Caveats / Not Found

- **codex 本机未安装**：`~/.codex/` 不存在、`codex` 不在 PATH → codex 结论全部来自官方文档 + Trellis 集成实证，未本机运行验证。残留不确定性见 `codex-skill-uncertainty.md`。
- **zcode 公开文档未找到**：zcode 约定来自本机官方 `zcode-plugins-official/skill-creator` SKILL.md（权威）+ Trellis 平台映射，未找到独立公开 zcode skill 文档站。
- **`.agents/skills/` 与 Claude Code**：Claude 官方位置表不含 `.agents/skills/`，本机靠 symlink 桥接。是否未来 Claude 原生读共享根未确认——当前按「Claude 需 `.claude/skills/` 副本或 symlink」设计最稳妥。
- **description 长度上限差异**：agentskills.io 标准/打包校验 = 1024；Claude Code 运行时列表截断 = description+when_to_use 合计 1536。bundled skill 按 **≤1024** 写最安全（满足标准与打包）。
- **allowed-tools 实验性**：各平台支持度不一，codex 官方未明确说明是否 honors；作为可选增强，不要依赖它做安全边界。
- clients.md（agentskills.io 客户清单）为 JS 渲染，静态抓取未取到列表；但 codex+claude 官方文档均显式引用 agentskills.io，zcode skill-creator 显式列 `.agents/skills/`，三方兼容性已三角验证。