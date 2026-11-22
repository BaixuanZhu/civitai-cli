# Research: codex Skill 集成 —— 已解决的核心问题 + 残留不确定性 + fallback

- **Query**: codex 是否原生支持 skills？目录约定？若不明给出最接近集成点与 fallback
- **Scope**: mixed（官方文档实证 + 本机缺失说明）
- **Date**: 2026-11-22

## 核心结论（初始不确定性已解决）

**codex 在 2026 年已原生支持 Agent Skills，遵循 agentskills.io 开放标准。** 不需要退回到「AGENTS.md / prompts / custom agents 等价物」。

- 官方文档：<https://developers.openai.com/codex/skills>（markdown 版 `…/skills.md`；repo `openai/codex/docs/skills.md` 仅指向此页）。
- 原文：「Use agent skills to extend ChatGPT and Codex with task-specific capabilities… Skills build on the open agent skills standard (agentskills.io)」。
- 「Standalone skills are available in the ChatGPT desktop app, **Codex CLI**, and IDE extension」。

### codex 读取位置（官方表，实证）
| Scope | 路径 | 说明 |
|---|---|---|
| REPO | `$CWD/.agents/skills` | 从 CWD 向上每层目录扫描 |
| REPO | `$CWD/../.agents/skills` | 父目录（嵌套仓库共享区） |
| REPO | `$REPO_ROOT/.agents/skills` | 仓库根（全仓共享） |
| USER | `$HOME/.agents/skills` | 用户级全局，跨仓库 |
| ADMIN | `/etc/codex/skills` | 机器/容器级（Linux 路径） |
| SYSTEM | 内置 | `skill-creator`、`skill-installer`，默认启用 |

**关键：codex 用 `.agents/skills/`，不是 `.codex/skills/`。** 官方位置表无 `.codex/skills/`。

### SKILL.md / 支持文件
- 必需 `name` + `description`；可选 `scripts/`、`references/`、`assets/`、`agents/openai.yaml`（外观+依赖）。
- 同名 skill **不合并**，都出现在选择器；支持 symlink（跟随 target）；自动检测变更（未出现则重启 Codex）。
- 调用：`/skills` 或 `$` 提及（CLI/IDE）；隐式靠 description 匹配。
- 渐进披露预算：初始列表 = name+description+**文件路径**，≤2% 上下文或 8000 字符；`skills.max_context_tokens` 上限 10000 tokens；skill 多时先缩短 description，必要时省略部分并告警。
- 启用/禁用：`~/.codex/config.toml` 的 `[[skills.config]]`（`path` 绝对路径匹配单文件 / `name` 匹配同名全部，`enabled=false`，最后匹配生效）；`[skills.bundled] enabled=false`（项目 trusted `.codex/config.toml`）。
- 安装 curated：`$skill-installer <name>`；对外分发用 plugins（ChatGPT/Codex 共享 universal plugin directory）。

### Trellis 集成佐证（本机实证）
- `.qoder/skills/trellis-meta/references/platform-files/platform-map.md:12`：`Codex | --codex | .codex/ | .agents/skills/ | .codex/agents/ | .codex/hooks/ + .codex/hooks.json`
- `…/local-architecture/bundled-skills.md:43`：Codex bundled skill root = `.agents/skills/<skill>/`
- `…/bundled-skills.md:66`：Codex 的 configure 创建「an intentionally empty `.codex/skills/` directory」——即 `.codex/skills/` 是**空占位**，真实 skill 写 `.agents/skills/`。
- `…/bundled-skills.md:62`：Codex/Gemini/Pi/Kimi 共享 `.agents/skills/`（upstream Agent Skills workspace alias）。

---

## 残留不确定性（明确标注）

| # | 不确定点 | 现状 / 影响 | 建议处理 |
|---|---|---|---|
| 1 | **本机无法运行验证** | `~/.codex/` 不存在、`codex` 不在 PATH（本机未装 codex）。结论全部来自官方文档 + Trellis 实证，未本机跑通 | 设计按官方文档；CI/有 codex 的机器上做一次 `civ skill install --platform codex` → `$skills` 可见性冒烟测试 |
| 2 | **`.codex/skills/` vs `.agents/skills/` 历史差异** | Trellis 仍创建空 `.codex/skills/`，疑似旧版 codex 曾读此目录或为兼容占位。官方当前表只列 `.agents/skills/` | **只写 `.agents/skills/`**；不要写 `.codex/skills/`（非官方读取位置，写了也不生效） |
| 3 | **skills 支持的版本门槛** | 官方文档未标注「自哪个 codex 版本起支持 skills」。旧版 codex 可能不读 `.agents/skills/` | install 时若检测到 codex 版本过旧，提示「需升级 codex 才能识别 skill」；不阻断写入 |
| 4 | **Windows 下 ADMIN 路径** | 官方 ADMIN = `/etc/codex/skills`（Linux 风格）。Windows 对应路径未文档化 | civ 只用 USER（`~/.agents/skills`）+ REPO（`.agents/skills`），**不碰 ADMIN/SYSTEM**，规避此不确定性 |
| 5 | **`allowed-tools` 是否被 codex honors** | agentskills.io 标注 allowed-tools 为实验性；codex 官方未明确说明支持 | bundled skill 的 `allowed-tools` 仅作可选增强，不依赖其做安全/免确认边界 |
| 6 | **codex 扩展 frontmatter 容忍度** | codex 官方只要求 name+description，未说明遇非标准字段（如 disable-model-invocation）是忽略还是报错 | bundled skill 严格只用标准六字段，规避风险 |
| 7 | **`agents/openai.yaml` 格式** | 官方仅说「appearance and dependencies」，未给完整 schema | 非必需，civ 首版不生成；如需 codex 专属外观/依赖再单独研究 |

---

## 建议 fallback（按优先级）

1. **首选（原生）**：写 `SKILL.md` 到 codex 原生根
   - 用户级：`~/.agents/skills/<name>/SKILL.md`
   - 项目级：`<repo>/.agents/skills/<name>/SKILL.md`
   - 这是官方支持路径，codex 装好后自动识别（`/skills` / `$<name>`）。

2. **共享根复用**：`.agents/skills/` 同时被 zcode 读取（zcode 发现优先级第 2/4 位）。若用户同时选 zcode+codex，一份 `.agents/skills/<name>` 可服务两者（zcode 另有 `.zcode/skills` 高优先 override，见主文件去重讨论）。

3. **codex 未安装时**：仍可写 `~/.agents/skills/`（无害，装后即读），但 install 应提示「未检测到 codex，skill 已写入共享目录，安装 codex 后生效」，或让用户确认是否跳过 codex。

4. **不推荐的等价物（仅作历史参考，当前无需）**：
   - `~/.codex/prompts/`（自定义 slash prompt）——codex 有原生 skills 后不必用。
   - `AGENTS.md` 引用——codex 读 `AGENTS.md` 作全局/项目指令，但那是「常驻指令」非「按需触发的 skill」，语义不同；仅在目标 codex 版本极旧、确无 skills 支持时才考虑作为降级，且需明确标注。
   - `.codex/agents/*.toml`（custom sub-agent）——是子代理定义，不是 skill 分发形态，不适用。

---

## 证据来源清单
- 官方：<https://developers.openai.com/codex/skills.md>（位置表、SKILL.md 要求、调用、预算、config、installer）
- repo：`openai/codex/docs/skills.md`（指向官方页，确认 skills 为正式文档主题）
- 标准：<https://agentskills.io/specification.md>
- 本机 Trellis：`.qoder/skills/trellis-meta/references/platform-files/platform-map.md:12,75-77`、`…/local-architecture/bundled-skills.md:43,62,66`、`…/platform-files/skills-and-commands.md:23`
- 本机缺失佐证：`ls ~/.codex/` → No such file or directory；`which codex` → not found

## 一句话给主代理
codex 原生支持 skills，装到 **`.agents/skills/`（用户级 `~/.agents/skills/`）**，不是 `.codex/skills/`；本机未装 codex 故未运行验证，残留点主要是版本门槛与 Windows ADMIN 路径——civ 只用 USER+REPO 共享根即可规避。