# Civitai CLI (civ) Go 工具开发

## Goal

基于规划文档（research/civitai-cli-plan.md，原件 I:/GitDownload/PixAI/civitai-cli-规划.md）从零开发 Go 标准库-only 的 Civitai 模型搜索/审卡 CLI（二进制名 `civ`），并参考 danbooru-tag-mcp 补齐发布基建：GitHub Actions CI/自动发布、集成测试、自更新、NSIS exe 安装包 + install.ps1 一键安装。

## 已确认决策（2026-11-22，用户拍板）

- GitHub 仓库坐标：`BaixuanZhu/civitai-cli`（自更新/install.ps1/release URL 均依赖）
- 平台范围：v1 Windows-only，amd64 + arm64（与参考项目一致；upgrade 资产名已按 GOOS/GOARCH 参数化，日后扩平台成本低）
- 参考范围：组织结构、CI/自动发布、集成测试、自更新、exe 安装包；**不参考**其响应缓存等深层业务设计（civitai 侧仅实现规划文档要求的 sync 哈希缓存）
- LICENSE：MIT（2026-11-22 用户确认，M1 骨架放入）

## Requirements

### 功能（规划文档 §4 为准）

- R1 `civ search <query>`：--type/--base/--sort/--period/--tag/--user/--limit；表格输出 + --json 原始 items 透传
- R2 `civ show <modelId>`：模型头 + 版本表；`-v <versionId>` 钻版本（文件表/全量哈希/扫描/触发词）；--prompts 例图配方（M3）
- R3 `civ vet <path|sha256|autov2>`：流式 SHA256 / by-hash 反查 / 审查报告；退出码 0 命中 / 3 未收录 / 1 用法 / 2 网络 / 4 token 失效
- R4 `civ sync <dir>`：递归扫描、哈希缓存（mtime+size 失效）、API 并发默认 3 上限 8、CSV/JSON 输出、dupe 标记、--rehash
- R5 `civ tags <query>`：tag 补全前 N 条
- R6 `civ login` / `civ logout`：favorites 探针校验、config.json 存储；token 优先级 --token > CIVITAI_API_KEY > 配置文件
- R7 `civ upgrade` / `civ version`：GitHub Release 自更新（checksums 校验 + Windows .bak 替换）、版本查询（参考项目机制，用户追加）
- R8 全局 flag：--json、--token、--timeout（默认 30s）、--verbose（调试日志走 stderr；规划文档的 -v 与 show -v 版本语义冲突，见 design.md 决策 D6）

### API 事实层约束（规划文档 §3/附录 A，2026-11-22 实测）

- R9 枚举含空格必须 url.Values 编码（裸空格 → 静默空响应，按可重试错误处理）
- R10 baseModel 白名单校验：非登记值警告（API 静默忽略未知值）
- R11 nsfwLevel 双形态：models 系列数字位掩码（json.Number），images 系列字符串，解码层分开
- R12 错误双形态兼容：ZodError 对象（400）与字符串 error（404/401）
- R13 重试：传输错误/空 body/429/5xx 退避重试（500ms→1s→2s + 抖动，封顶 8s，尊重 Retry-After）；400/401/403/404 不重试
- R14 宽容解码：未知字段忽略、未知枚举原样保留、stats 可空、meta 可为 null（hasMeta=true 不保证非空）
- R15 默认匿名，token 可选增强；不绕过 NSFW 门控

### 基建（参考 danbooru-tag-mcp，research/danbooru-infra.md）

- R16 CI：push/PR 单元测试（vet/test/race/build，windows-latest，go-version-file）；每周定时 + 手动真实外网集成测试（永不阻塞发布路径）
- R17 发布：tag v* 触发 → 测试门禁 → ldflags 版本注入 → amd64/arm64 构建 → NSIS setup.exe ×2 + 确定性 zip ×2 + checksums.txt + install.ps1 上传 → attestation → verify-repro 跨平台重建比对
- R18 自更新：releases/latest 检查、版本比较（本地更新跳过）、zip 下载 + SHA256 校验、同卷 .bak 改名替换、启动清理残留 .bak
- R19 安装：per-user NSIS 免 UAC（$LOCALAPPDATA\Programs\civitai-cli）、PATH 委托 exe 自举（HKCU\Environment + WM_SETTINGCHANGE 广播，dev 构建 ldflags 关闭）、install.ps1 全 ASCII 无 BOM 一键安装
- R20 可复现构建：go.mod 钉死 toolchain、CGO_ENABLED=0 -trimpath -buildvcs=false、确定性 zip（SOURCE_DATE_EPOCH）、CHANGELOG Keep-a-Changelog 标题机器契约
- R21 注册表写入类测试（env PATH 自举）必须 CIV_TEST_REGISTRY=1 门控且**只在 GitHub Actions 隔离 VM 执行，禁止本机运行**（用户既定规则）

### 非目标（v1 明确不做，规划文档 §1）

- 模型/图片下载、上传/创作者写操作、本地登记册格式解析、绕过 NSFW 门控、API 响应缓存（CDN 已有 5 分钟缓存）、/images 浏览端点（无 v1 命令消费，--prompts 走版本详情 images[]）

## 任务地图（子任务顺序执行）

| 子任务 | 交付物 | 独立验收 |
|---|---|---|
| 11-22-civ-m1-core | 骨架 + api client + search/show/tags + fixture + 单元测试 | anima/stocking 出表，--json 可解析，测试零外网 |
| 11-22-civ-m2-vet | vet 命令 + 流式哈希 + 退出码 | 本地文件/哈希反查出报告，404 → exit 3 |
| 11-22-civ-m3-sync-login | sync + hashcache + login/logout + --prompts | 50 文件 ≤40s，缓存二次 ≤5s，token 探针 |
| 11-22-civ-m4-release | Makefile 完整 + CI/release + upgrade + env + NSIS + install.ps1 + zipfile + CHANGELOG + 集成测试脚本 | make dist-all 产出 4 产物 + checksums；upgrade 单测全绿 |
| 11-22-civ-m5-docs | README + docs/plan.md 迁移 + 最终验收 | README 含全部命令示例与安装三选项；§10 验收全核对 |

## Acceptance Criteria（跨子任务，父任务最终集成审查）

- [ ] `go build` 单文件静态二进制，CGO 禁用，go.mod 零第三方依赖
- [ ] 全部命令支持 --json，输出契约稳定；人类输出 stdout、日志 stderr
- [ ] 退出码语义文档化且稳定：0 成功/命中、1 用法、2 网络/重试耗尽、3 未收录（vet）、4 token 失效
- [ ] 单元测试零外网（httptest + testdata fixture）；`go vet` / `go test -race` 全绿
- [ ] vet 单文件 < 3s（缓存命中 < 1s）；sync 50 文件 ≤ 40s、二次 ≤ 5s（真实目录可用时实测，否则合成基准 + 文档注明）
- [ ] make dist-all 产出 2 setup.exe + 2 zip + checksums.txt；zip 字节级可复现
- [ ] CI 工作流语法正确、unit-test 路径不触网；release 工作流 tag 触发链完整（真实发布需用户推 tag 后验证）
- [ ] 自更新逻辑单测覆盖（版本比较/checksum 解析/.bak 替换回滚/单文件解压）；注册表测试仅 CI 门控执行
- [ ] README 含安装三选项、全部命令示例、退出码表、环境变量表、可复现构建说明

## Open Questions

- 性能验收的真实 LoRA 目录路径：M3 验收时向用户要（规划文档示例 I:\ComfyUI-Workspace\models\loras）

## Notes

- 需求事实源：research/civitai-cli-plan.md（API 实测规范附录 A-D）
- 基建设计源：research/danbooru-infra.md、research/danbooru-code-org.md
- 技术设计见 design.md；子任务执行顺序与集成审查见 implement.md