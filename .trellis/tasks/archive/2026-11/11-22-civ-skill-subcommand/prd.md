# civ skill 子命令 + bundled 提示词创作 Skill

## Goal

新增 `civ skill install/remove/update` 子命令，把随版本分发的「文生图提示词创作」Skill（`civ-prompt`，go:embed 嵌入二进制）安装到 zcode / Claude Code / codex 三平台的 skill 目录，让编码 agent 在提示词创作时用 civ 的触发词 + 例图配方数据驱动 prompt 组装。

## 已确认决策（2026-11-22，用户拍板）

- 走 Trellis 任务 + 规划（复杂功能）
- install **交互式可选**：无 flag 时交互选择平台 + scope；可用 flag 非交互指定
- 目标平台：zcode / Claude Code / codex（OpenAI Codex CLI）
- Skill 随版本分发（嵌入二进制，任何安装好的 civ 都能 skill install）

## Requirements

### Skill 内容（civ-prompt）

- R1 可移植 SKILL.md：仅 agentskills.io 标准六字段（name/description/license/compatibility/metadata/allowed-tools），**不用平台扩展字段**（when_to_use/disable-model-invocation 等会破坏打包校验）
- R2 name=`civ-prompt`（kebab-case，=目录名）；description 单行 ≤1024，含文生图触发关键词（文生图/提示词/prompt/Stable Diffusion/Flux/SDXL/Illustrious/Anima/LoRA/触发词/负向提示词）+ 何时用
- R3 正文教 agent：本地文件→`civ vet <path> --prompts --json`；已知 ID→`civ show <id> -v <vid> --prompts --json`；找卡→`civ search <concept> --type lora --json`；提取触发词（trainedWords，激活 LoRA 必需，逐字纳入）+ 例图配方（prompt/negativePrompt/steps/cfgScale/sampler/scheduler）组装最终 prompt
- R4 body <500 行；长内容拆 references/（civ 命令速查 + JSON 形状、prompt 组装模式与陷阱）；metadata.version=civ 版本（供 update 比对）；license MIT
- R5 无冗余文件（不放 README/CHANGELOG）；无 TODO 残留

### skill 子命令

- R6 `civ skill install`：无 flag→交互（探测已装平台，多选平台 + scope，默认 user-level，确认）；`--platform claude,zcode,codex|all` + `--scope user|project` + `--force` 非交互
- R7 `civ skill remove`：删目标平台 root/civ-prompt/ 整目录；同样支持交互/flag 选平台+scope
- R8 `civ skill update`：比对已装 metadata.version vs 二进制 app.Version，不同则保留原名覆盖，相同则报 up-to-date；--force 强制重写
- R9 平台→root 映射（研究实证）：claude user `~/.claude/skills/` project `.claude/skills/`；zcode user `~/.zcode/skills/` project `.zcode/skills/`；codex user `~/.agents/skills/` project `.agents/skills/`（**codex 绝不写 .codex/skills/**）
- R10 各平台写各自原生根（真实副本，**不依赖 symlink**——Windows 需特权）；mkdir -p 整树 + 写 SKILL.md（渲染版本）+ references/
- R11 冲突处理：目标已存在→读 metadata.version 判断→交互 overwrite/skip/backup(.bak)，--force 覆盖；**不动 zcode .skills_store_lock.json**
- R12 平台探测：~/.claude/ ~/.zcode/ ~/.codex/ ~/.agents/ 目录存在性 + PATH 二进制；codex 未装仍可写 ~/.agents/skills/（装后即读）但提示
- R13 输出：人类可读摘要（装到哪些路径）+ --json 结构化；进度/警告 stderr；纯本地文件操作无网络
- R14 退出码：0 成功 / 1 用法错误（无效平台/scope/交互取消）；无网络类

### 分发与集成

- R15 go:embed 嵌入 skill 树（SKILL.md 模板 + references/）进二进制；install 时渲染 metadata.version=app.Version
- R16 Skill 源文件入仓（随版本分发，repo 内可见可改）；release 产物（zip/setup.exe）经二进制携带 Skill，无需额外打包
- R17 main.go 增 `case "skill"`；usage() 更新；README 增 skill 章节（install/remove/update + 三平台 + 交互/flag）；CHANGELOG 条目

### 测试

- R18 零外网单测：平台 root 映射表驱动、版本比对、冲突处理（临时目录）、install/remove/update（t.Setenv HOME/USERPROFILE 隔离）、embed FS 含 SKILL.md、版本渲染、交互（stdin 注入）+ flag 模式、--json
- R19 SKILL.md 校验测试：name=目录名、description 单行 ≤1024、body <500 行、仅标准六字段、无 TODO

## Acceptance Criteria

- [ ] `civ skill install --platform claude --scope user` 把 civ-prompt 装到 ~/.claude/skills/civ-prompt/（SKILL.md + references/，metadata.version=当前 civ 版本）
- [ ] `civ skill install`（无 flag）交互可选平台+scope，探测已装平台预选
- [ ] zcode/codex 分别装到 ~/.zcode/skills/ 与 ~/.agents/skills/（codex 不写 .codex/skills/）
- [ ] `civ skill update` 版本相同报 up-to-date，不同覆盖保留原名；`civ skill remove` 删整目录
- [ ] 冲突交互 overwrite/skip/backup，--force 覆盖；不动 zcode lock 文件
- [ ] SKILL.md 通过校验（标准六字段、name=civ-prompt、description≤1024 单行、body<500 行、无 TODO）
- [ ] go vet/test/race 全绿零外网；--json 可解析
- [ ] Skill 嵌入二进制（make build 后 civ skill install 无需仓库源文件即可装）
- [ ] README skill 章节 + CHANGELOG 条目；main.go/usage 接线
- [ ] 至少一个平台真实 install→agent 可见性冒烟（claude/zcode 本机可验；codex 本机未装，注明 CI/有 codex 机器补验）

## Notes

- 研究依据：research/platform-skill-conventions.md（三平台对照 + agentskills.io 规范 + 设计建议）、research/codex-skill-uncertainty.md
- Skill 内容设计遵循 skill-creator 指南（简洁/渐进披露/imperative），但 frontmatter 用可移植标准六字段（研究约束优先于 skill-creator 的 Qoder 扩展字段）
- 技术设计见 design.md；执行清单见 implement.md