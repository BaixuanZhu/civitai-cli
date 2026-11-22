# 技术设计 — civ skill 子命令 + civ-prompt Skill

> 需求见 prd.md；平台约定实证见 research/platform-skill-conventions.md。全局编码约定见 .trellis/spec/backend/（已填实）。

## 1. 架构与目录

```
internal/skill/
├── embed.go          # //go:embed assets/civ-prompt → embed.FS；SkillName 常量
├── skill.go          # Platform/Scope 类型、RootFor 映射、DetectPlatforms、版本渲染
├── install.go        # Install/Remove/Update 核心逻辑（纯文件操作，可测）
├── assets/civ-prompt/
│   ├── SKILL.md      # 模板（metadata.version: "__CIV_VERSION__" 占位）
│   └── references/
│       ├── civ-commands.md    # 命令速查 + --json 形状
│       └── prompt-patterns.md # 触发词/配方组装模式与陷阱
└── *_test.go
internal/cmd/skill.go  # skill 子命令：flag/交互 → 调 internal/skill → 渲染输出
```

依赖方向：main → cmd/skill → internal/skill（叶子包，embed + 文件操作，不依赖 api）。复用 internal/app（Version）、internal/report（json 输出）、cmd/globals（execute runner/全局 flag）。

## 2. 平台映射与探测（research 实证）

```go
type Platform string // "claude" | "zcode" | "codex"
type Scope string    // "user" | "project"
func RootFor(p Platform, s Scope) (string, error)
// claude: user ~/.claude/skills  project <cwd>/.claude/skills
// zcode:  user ~/.zcode/skills   project <cwd>/.zcode/skills
// codex:  user ~/.agents/skills  project <cwd>/.agents/skills  （绝不 .codex/skills）
```

- user 根用 os.UserHomeDir()；project 根用 os.Getwd()（调用时 cwd）。skill 目录 = RootFor + "/" + SkillName。
- DetectPlatforms()：检查 ~/.claude/ ~/.zcode/ ~/.codex/ ~/.agents/ 存在性 + exec.LookPath("claude"/"zcode"/"codex")。返回已装平台集（交互预选）。codex 未装但 ~/.agents/ 存在仍可写（提示）。
- 各平台写各自原生根（真实副本，不用 symlink）。zcode+codex 同选时各写 .zcode/skills 与 .agents/skills（zcode 会看到两份，.zcode 优先——文档说明，行为最可预测）。

## 3. 嵌入与版本渲染

- embed.go：`//go:embed assets/civ-prompt` → `var FS embed.FS`。SkillName="civ-prompt"。
- SKILL.md 模板 frontmatter `metadata: {version: "__CIV_VERSION__"}`。Install/Update 时读 embed SKILL.md，strings.Replace("__CIV_VERSION__", app.Version) 渲染后写目标。references/ 原样复制。
- 版本比对：读已装目标 SKILL.md 的 metadata.version（解析 YAML frontmatter——零依赖手写极简 frontmatter 提取，或正则取 version 行；不引 yaml 库）。与 app.Version 比对决定 update 是否重写。
- 决策：frontmatter 解析只用极简提取（metadata.version 单行），不引第三方 YAML（零依赖约束）。SKILL.md 格式受控（我们自己写），正则/行扫描足够。

## 4. install/remove/update 逻辑（internal/skill/install.go）

```go
type Options struct { Platforms []Platform; Scope Scope; Force bool; Out io.Writer }
type Result struct { Platform Platform; Path string; Action string } // installed/updated/skipped/removed/backed-up
func Install(opts) ([]Result, error)
func Remove(opts) ([]Result, error)
func Update(opts) ([]Result, error)
```

- Install：每平台 mkdir -p root/civ-prompt/ → 若已存在且非 Force：读 metadata.version，交互模式问 overwrite/skip/backup（cmd 层注入决策回调），非交互默认 skip（除非 --force）。写入渲染 SKILL.md + references/（embed FS walk）。backup=改名 .bak。
- Remove：os.RemoveAll(root/civ-prompt/)（不存在→skipped/no-op）。**不触碰 .skills_store_lock.json**。
- Update：读已装 version vs app.Version；相同→up-to-date skip；不同→保留原名覆盖（同 Install 写入）。不存在→提示先 install（或视为 install）。
- 冲突决策回调：`type ConflictFunc func(path, installedVersion string) (action string)`，cmd 交互模式传询问函数，非交互传默认（skip/force-overwrite）。保持 internal/skill 纯逻辑可测（不读 stdin）。

## 5. cmd/skill.go 与交互 UX

- `civ skill <install|remove|update> [flags]`；子命令解析（skill 后第一个位置参数是动作）。
- flags：--platform（逗号分隔 claude,zcode,codex 或 all）、--scope（user|project，默认 user）、--force、--json（全局）。
- 无 --platform → 交互：DetectPlatforms 预选 → stderr 提示编号多选平台（stdin 读，复用 login 的 stdin 参数化模式）→ scope 选择（默认 user）→ 确认。交互取消→exit 1。
- 有 --platform → 非交互直接执行。--scope 缺省 user。
- 输出：人类摘要（每平台 action + path）；--json []Result。进度/警告 stderr。
- 退出码：0 成功 / 1 用法（无效平台/scope/未知动作/交互取消）。纯本地无网络。
- main.go：`case "skill": cmd.Skill(os.Args[2:])`；usage 增 skill 行。

## 6. civ-prompt SKILL.md 内容设计（skill-creator 原则 + 可移植六字段）

frontmatter（仅标准六字段）：
```yaml
name: civ-prompt
description: <单行≤1024，文生图提示词创作 + 触发关键词 + 何时用>
license: MIT
metadata:
  version: "__CIV_VERSION__"
```
（allowed-tools 可选 `Bash(civ:*)` 实验性，不依赖；compatibility 可省）

body（imperative，<500 行）：
- Overview：用 civ CLI 的真实 Civitai 数据（触发词 + 例图配方）驱动文生图 prompt 组装，而非凭空写。
- When to use：创作 SD/Flux/SDXL/Illustrious/Anima prompt、选 LoRA、需触发词或别人怎么用的配方。
- Workflow：① 定位模型（本地文件 vet / 已知 ID show -v / 概念 search）② 加 --prompts --json 取触发词 + 配方 ③ 组装：触发词逐字纳入、采纳配方 steps/cfg/sampler/scheduler 与 prompt 结构、负向 prompt 参考例图 ④ 输出最终 prompt+negative+推荐参数+触发词块。
- Key facts：触发词必须逐字包含才能激活 LoRA；配方是真实生成（有据）；匿名可用无需 token；--json 供解析；civ 不下载模型/图、不生成图（只提供数据）。
- Resources：references/civ-commands.md（命令+JSON 形状速查）、references/prompt-patterns.md（组装模式/陷阱）。
- 边界：不臆造触发词；meta null 的例图跳过；nsfw 匿名钳制说明。

references/civ-commands.md：search/show/vet/sync/tags 命令 + --json 输出形状（vet keys hash/version/model/examples；--prompts keys prompts/skipped）+ 退出码。
references/prompt-patterns.md：触发词放置（通常 prompt 开头）、LoRA 权重语法 `<lora:name:weight>`（ComfyUI/A1111 差异）、配方参数采纳、负向 prompt 复用、常见陷阱（漏触发词/过度堆砌/底模不匹配）。

## 7. 测试策略

- internal/skill：RootFor 表驱动（三平台×两 scope）；DetectPlatforms（临时 HOME）；版本渲染（__CIV_VERSION__→app.Version）；Install/Remove/Update（t.Setenv HOME/USERPROFILE 临时目录，断言文件写入/删除/版本 skip/overwrite/backup）；冲突回调；embed FS 含 SKILL.md+references。
- SKILL.md 校验测试：name=civ-prompt=目录名；description 单行 ≤1024 无尖括号；body <500 行；frontmatter 仅标准六字段（无 when_to_use 等）；无 TODO。
- cmd/skill：交互（stdin strings.Reader 注入）+ flag 模式；install 到临时平台根；remove；update 版本 skip/overwrite；--json 解析；无效平台/scope→exit 1。
- 零外网（纯文件操作）；零依赖（手写 frontmatter version 提取，不引 yaml）。

## 8. 分发与兼容

- embed 使二进制自带 Skill；make build/release 链不变（二进制携带）。repo 内 assets/civ-prompt/ 是源（随版本分发，可改）。
- 向后兼容：skill 子命令是新增，不影响现有命令。旧二进制无 skill 子命令（unknown→exit 1）。
- 回滚：skill 安装是文件写入，remove 即回滚；二进制回滚=revert commit。
- codex 本机未装：install 写 ~/.agents/skills/ 逻辑可测（临时 HOME），真实可见性冒烟留 CI/有 codex 机器（prd 注明）。