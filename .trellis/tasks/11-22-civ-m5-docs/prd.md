# M5 README 与最终验收

## Goal

交付面向用户的 README、迁移规划文档入仓、完成规划文档 §10 验收标准全量核对与父任务集成审查。

## Requirements

### README.md（英文，参考项目结构）

- 简介（找卡/审卡定位）+ Quick start（civ search / civ vet 两例）
- Features（六命令 + 自更新 + 零依赖单文件）
- Install 三选项：setup wizard（NSIS per-user 免 UAC）/ one-line PowerShell（iwr install.ps1 | iex，含 CIV_INSTALLER_MIRROR 说明）/ portable zip
- Commands：search/show/vet/sync/tags/login/logout/upgrade/version 全示例（可复制执行）+ 全局 flag 表（--json/--token/--timeout/--verbose，注明 -v 冲突决策 D6）
- Exit codes 表（0/1/2/3/4 语义 + sync not_found 是数据行说明）
- Token & config：优先级三层、config 路径、login 探针、匿名能力边界（NSFW 门控说明）
- Environment variables 表：CIVITAI_API_KEY / CIV_NO_BOOTSTRAP / CIV_NO_CACHE / CIV_TEST_REGISTRY（注明仅 CI）/ CIV_INSTALLER_MIRROR
- Hash cache：路径、失效判定、--rehash
- Building from source：make 目标表、Go toolchain 钉死说明
- Reproducible builds & verifying downloads：checksums/attestation/verify-repro 说明
- API 事实层注意事项：CDN 5 分钟缓存、baseModel 白名单警告、匿名 NSFW 钳制（规划文档 §9 风险表的用户侧摘要）

### docs/plan.md

- 从 research/civitai-cli-plan.md 迁移（原文保留中文，头部加"已实现状态"注记：v1 范围/偏差决策 D6/省略 images.go 等）

### 最终验收（父任务 prd.md 跨子任务清单）

- 规划文档 §10 逐条核对表（性能项实测或合成基准 + 注明）
- 全命令 --json 契约抽查、退出码矩阵抽查（0/1/2/3/4 各一例）
- go.mod 零 require、CGO_ENABLED=0 单文件确认
- LICENSE 已确认 MIT（M1 骨架放入，M5 核对存在与版权行）
- .trellis/spec/backend 填入本项目实际约定（directory-structure/error-handling/quality-guidelines/logging：从父 design.md 与参考约定提炼，英文）

## Acceptance Criteria

- [ ] README 全部命令示例可复制执行（live 抽查或 fixture 说明）；安装三选项与真实产物名一致
- [ ] docs/plan.md 入仓且偏差决策有注记
- [ ] 规划文档 §10 验收核对表完成（每项 ✓/注明）
- [ ] 退出码矩阵 5 例抽查通过；--json 全命令 python 解析通过
- [ ] spec/backend 至少 directory-structure 与 error-handling 填实（英文）
- [ ] LICENSE（MIT）存在且版权行正确
- [ ] 父任务集成审查清单全勾 → 父任务归档提醒

## Notes

- 轻量任务：PRD + implement 清单；无新技术设计
- README 语言英文（参考项目约定 + 开源受众）；docs/plan.md 保留中文原文