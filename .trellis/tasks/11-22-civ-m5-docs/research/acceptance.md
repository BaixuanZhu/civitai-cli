# M5 最终验收核对表 — civitai-cli

> 任务：`.trellis/tasks/11-22-civ-m5-docs`（README + docs/plan.md 迁移 + spec 填实 + 最终验收）
> 核对时间：2026-11-22
> 核对二进制：`dist/amd64/civ.exe`（`civ 0.1.0-dev`，dev 构建 Bootstrap=off）
> 环境：所有 live 运行设 `CIV_NO_BOOTSTRAP=1`；sync 契约抽查设 `CIV_NO_CACHE=1`（避免写真实缓存）；临时合成目录用完已删除
> 工具链：`go1.27.3 windows/amd64`；python 3.14.4（`--json` 解析）
> 图例：✓ = 实测通过；✓(条件) = 通过但注明条件；⏳ = 推迟主代理（注明条件）

---

## 1. 规划文档 §10 验收标准逐条核对

| # | §10 标准 | 结论 | 证据 / 条件 |
|---|---|---|---|
| 1 | `vet` 单文件 < 3s（缓存命中 < 1s） | ✓(条件) | live 已知哈希 `vet FBA1195327` 三次计时 **1.861s / 1.680s / 1.854s**（均 < 3s）。条件：① 哈希输入**不含本地哈希计算**；本地文件 vet 额外含流式 SHA256 时间，∝ 文件大小（如 3.9 GB Anima 文件仅哈希就需数秒）——真实大文件计时推迟主代理。② "缓存命中 < 1s" 指 **CDN 5 分钟 HIT**（`s-maxage=300`）；`vet` 不走 hashcache（每次新鲜计算），三次均 ~1.8s 提示 origin/MISS 或双 API 调用（by-hash + 取所属 model）开销。 |
| 2 | `sync` 50 文件 ≤ 40s，二次运行 ≤ 5s | ⏳(条件) | **真实 LoRA 目录性能计时推迟主代理**（需用户路径，规划示例 `I:\ComfyUI-Workspace\models\loras`）。条件注明：二次 ≤ 5s 依赖 **hashcache 命中**（size+mtime 未变 → 跳过重算）**+ CDN 5min HIT**；计时须**开缓存**（`CIV_NO_CACHE=1` 会禁用缓存，仅用于契约抽查/漂移金丝雀）。首跑 ≤ 40s 理论达标：并发 3 × origin ~1.3–2s/call，50 文件 ≈ 17 批 × ~1.8s ≈ 30s。合成基准可行（50 合成文件 → 50 次 live by-hash，全 not_found），本次未跑以避大量外网调用。 |
| 3 | 全部命令支持 `--json`，输出契约稳定 | ✓ | 见 §3 全命令契约抽查（search/show/tags/vet/sync 各一次 python `json.load` 通过；另抽查 `--prompts` 配方视图）。 |
| 4 | 测试不访问外网（httptest + fixture） | ✓ | `go test ./...` 与 `go test -race -count=1 ./...` 全绿（离线）；api 层 httptest fake server，cmd 层经 `newAPIClient`/`newSyncClient`/`computeHash` seam 指向 httptest；`testdata/*.json` 为线上 verbatim fixture（FIXTURES.md 记抓取日期）；注册表测试本机 `t.Skip`。 |
| 5 | 退出码语义文档化且稳定 | ✓ | README "Exit codes" 表（0/1/2/3/4 + sync not_found 数据行说明）；live 实测 0/1/3，单测覆盖 2/4（见 §2）。 |
| 6 | `go build` 单文件二进制，无 CGO | ✓ | `go version -m dist/amd64/civ.exe`：`CGO_ENABLED=0`、`-trimpath=true`、`-buildmode=exe`、`GOOS=windows GOARCH=amd64`、`go1.27.3`；单文件 7.34 MB。 |

---

## 2. 退出码矩阵抽查（0/1/2/3/4 各一例）

| 码 | 语义 | 抽查方式 | 证据 |
|---|---|---|---|
| 0 | 成功/命中 | **live** | `civ search anima --limit 2` → exit 0；`civ vet FBA1195327` → exit 0（命中报告） |
| 1 | 用法错误 | **live** | `civ search anima --limit 101` → exit 1（`--limit must be 1-100`）；`civ bogus`（未知命令）→ exit 1；`civ vet Z:/no/such/file` → exit 1 |
| 2 | 网络/重试耗尽 | **单测为据** | `TestExitCodeFor`（globals_test.go:21 network→2）；`TestLogin_NetworkExit2NoSave`（login_test.go:73，httptest 500 + 短 backoff → NetworkError → 2）；`TestClient_RetryExhaustion`（client_test.go:192）。live 不引入测试专用 flag/断网，故以 httptest 单测为据。 |
| 3 | 哈希未收录（vet） | **live** | `civ vet DEADBEEF00`（10 hex → AutoV2 → by-hash 404）→ exit 3 + vet 专属 hint（"this hash is not registered on Civitai..."）；`TestVetExitCode`/`TestVet_NotFoundMapsToExit3` 佐证 |
| 4 | token 失效 | **单测为据** | `TestExitCodeFor`（unauth+token→4）；`TestVetExitCode`（unauth+token→4）；`TestLogin_Rejected401Exit4NoSave`（login_test.go:51，httptest 401 → UnauthorizedError → loginExitCode 4，且不写 config）。live 无有效 token，故以 httptest 单测为据。 |

补充语义核对（单测 `TestExitCodeFor`/`TestVetExitCode`）：匿名 401 → 1（需 token，非 4）；404 在 search/show/tags → 1，仅 vet → 3；sync 的 not_found 是数据行 → 整体 exit 0（live 合成目录抽查佐证，见 §3）。

---

## 3. 全命令 `--json` 契约抽查（python `json.load`）

| 命令 | 调用 | 解析结果 | civ exit |
|---|---|---|---|
| search | `civ search anima --limit 3 --json` | `list`，len=3（原始 items 数组透传） | 0 |
| show | `civ show 2458426 --json` | `dict`，id=2458426（模型对象） | 0 |
| tags | `civ tags anima --limit 5 --json` | `list`，len=5（名称数组） | 0 |
| vet | `civ vet FBA11953...F7EB --json` | `dict`，keys=`hash,version,model,examples` | 0 |
| sync | `civ sync <临时合成目录> --json`（`CIV_NO_CACHE=1`） | `list`，1 行 `status=not_found`、`triggers=0` | 0 |
| show --prompts | `civ show 2458426 -v 3263843 --prompts --json` | `dict`，keys=`prompts,skipped`，prompts=9、skipped=0 | 0 |
| vet --prompts | `civ vet FBA11953...F7EB --prompts --json` | `dict`，keys=`prompts,skipped`，prompts=9 | 0 |

- sync 另抽查 `--format csv`：表头 `FILE,STATUS,MODEL,VERSION,BASE,TRIGGERS,PICKLE,VIRUS,MODEL_URL` + not_found 行，exit 0。
- **stdout/stderr 纪律核对**：sync 进度 `[INFO] sync 1/1` 走 stderr（stdout 仅 JSON/CSV）；baseModel 告警走 stderr（`log.go:43` = `app.Warnf`），结果走 stdout。
- 临时合成目录（`mktemp -d` + 256 字节随机 `.safetensors` + 一个 `.txt`）用完 `rm -rf` 已删除确认。

---

## 4. 零依赖 / CGO / 单文件确认

- `go.mod`：**0 个 require**（仅 `module civitai-cli` / `go 1.27` / `toolchain go1.27.3`）。
- `go list -m all` → 仅 `civitai-cli`（零第三方模块）。
- `go list -deps .` 中的 `vendor/golang.org/x/...`、`crypto/internal/...` 是 **Go 工具链自带的 stdlib 内部 vendoring**（net/http、crypto/tls 使用），非外部模块依赖。
- 已构建 exe 元数据：`CGO_ENABLED=0`、`-trimpath=true`、`-buildvcs=false`（无 vcs 戳记行）、`-buildmode=exe`、单文件。

---

## 5. LICENSE 确认

- `LICENSE` 存在，**MIT License**，版权行 **`Copyright (c) 2026 BaixuanZhu`**（与父任务已确认决策一致）。README 末尾链接一致。

---

## 6. go vet / go test / go test -race 全绿

| 命令 | 结果 |
|---|---|
| `go vet ./...` | exit 0（无输出） |
| `go test ./...` | 全 ok（api/app/cmd/config/env/hashcache/report/upgrade/scripts/zipfile） |
| `go test -race -count=1 ./...` | 全 ok（非缓存，~1.2–1.9s/包） |
| `go test -run TestPersistRoundtrip -v ./internal/env/` | **SKIP**（`CIV_TEST_REGISTRY` 未设 → 本机正确跳过，符合"仅 CI 隔离 VM"规则） |

---

## 7. 产物名一致性核对（文档 ↔ 实现）

| 来源 | 产物名 | 一致 |
|---|---|---|
| Makefile（`PRODUCT=civitai-cli`，zip/installer target） | `civitai-cli-windows-<arch>.zip`、`civitai-cli-windows-<arch>-setup.exe` | ✓ |
| installer/civitai-cli.nsi（`OutFile`） | `..\dist\civitai-cli-windows-${APP_ARCH}-setup.exe` | ✓ |
| install.ps1（`$zipName`） | `civitai-cli-windows-$assetArch.zip` | ✓ |
| internal/upgrade（`expectedAssetName`） | `civitai-cli-%s-%s.zip`（GOOS/GOARCH） | ✓ |
| .github/workflows/release.yml（checksums/attestation/upload） | 4 产物 + checksums.txt + install.ps1 | ✓ |
| 本地 dist/ 现状 | `civitai-cli-windows-amd64-setup.exe`、`civitai-cli-windows-amd64.zip`、`checksums.txt`（仅列 amd64.zip）、`amd64/civ.exe` | ✓(条件) |
| README 安装三选项 / 自更新 / 可复现章节 | 与上述产物名逐字一致 | ✓ |

条件注明：本地 dist 仅 **amd64**（`make release` 的 checksums 只 `sha256sum *.zip`，故本地 checksums.txt 仅含 zip）；**arm64 与 4 产物完整 checksums 由 CI release.yml 链产出**（`make build-dist/installer-only/zip GOARCH=arm64` + sha256sum 全 4 产物）。zip 字节级可复现由 `scripts/zipfile`（`SOURCE_DATE_EPOCH` 钉 entry 头）保证，`scripts/zipfile/main_test.go` 覆盖；双架构 `make dist-all` 全量产出属 M4 门禁。

---

## 8. 父任务集成审查清单交叉核对（父 prd.md Acceptance Criteria）

| 父验收项 | 结论 | 备注 |
|---|---|---|
| go build 单文件静态、CGO 禁用、go.mod 零第三方 | ✓ | §4 |
| 全命令 --json 契约稳定；人类 stdout、日志 stderr | ✓ | §3 |
| 退出码语义文档化稳定 0/1/2/3/4 | ✓ | §2 + README |
| 单元测试零外网；go vet / go test -race 全绿 | ✓ | §6 |
| vet <3s（缓存<1s）；sync 50≤40s、二次≤5s | ✓(条件)/⏳ | §1 项 1/2：vet 哈希输入实测达标；sync 真实目录计时推迟主代理 |
| make dist-all 产 2 setup + 2 zip + checksums；zip 字节可复现 | ✓(条件) | §7：本地 amd64 产物在；双架构由 CI release 链；确定性 zip 单测覆盖 |
| CI 工作流语法正确、unit-test 不触网；release tag 链完整 | ✓(条件) | 已读 ci.yml/release.yml 审查：unit-test 仅 vet/test/race/build（不触网），online-smoke 仅 schedule/manual；release tag→测试门禁→双架构→installer/zip→checksums→attestation→awk notes→gh-release→verify-repro 链完整。**actionlint 未跑**（本机未装，以人工语法审查为据）；真实 tag 发布留用户（外部可见动作） |
| 自更新单测覆盖（版本比较/checksum/.bak 回滚/单文件解压）；注册表测试仅 CI 门控 | ✓ | upgrade_test.go：`TestCompareVersions`/`TestParseChecksum`/`TestReplaceExeAt_RollbackOnMoveFailure`/`TestExtractSingleExe`(+MissingExe)/`TestCleanupBakAt`/`TestExpectedAssetName`/`TestDownloadOnce`(+Non200)；env 注册表测试本机 SKIP（§6） |
| README 含安装三选项、全命令示例、退出码表、环境变量表、可复现构建 | ✓ | README.md 已交付，示例经 live 抽查可复制执行 |

---

## 9. 文档与实现一致性发现（核对代码所得）

1. **`baseModel` 单数参数已死**：实现改用复数 `baseModels` 且仅首值生效（`api/models.go`、`cmd/search.go resolveBaseFilters`）。规划草案 §3.3-2/附录 A.1 的"逗号多值"已过时——docs/plan.md 头部注记 4 与 README "API notes" 均已更正。live 抽查：`--base Illustrious` 过滤生效；未知值 `[WARN] not in the verified platform list`；多值 `[WARN] the API applies only the first baseModel filter`。
2. **`--sort recently-added` 需 collectionId**：枚举表保留该值，但 plain `/models` 搜索用它返回 HTTP 400（`api/enums.go` 注释 + FIXTURES.md 项 4）。README search 表已加脚注；`search.go` 的 `--sort` flag help 字符串**未列 recently-added**（仅列其余 9 值）——属帮助文案与枚举表的轻微不一致，不影响功能（映射表含该值，错误信息会列全）。**建议主代理后续补 help 文案**（`internal/cmd/search.go` 本任务禁改）。
3. **陈旧 hint 文案**：`internal/cmd/globals.go:137` `printErrorHint` 对"带 token 的 401"仍打印 `(civ login arrives in M3)`，但 login 已于 M3 实现。该 hint 仅在 search/show/tags/sync 带 token 被拒（exit 4）时出现（vet/login 各自 override hint）。**属代码内陈旧字符串，本任务禁改 internal/*，提请主代理修正为指向 `civ login`**。
4. **D6 -v 冲突**：实现与决策一致——全局 `--verbose` 无短别名，`show -v/--ver` 版本语义，`version` 支持 `-v/--version`。README 已注明。
5. **省略 images.go / hashcache 兼放 ComputeSHA256**：与 design.md §1 差异决策一致；docs/plan.md 注记 2/3 已注明。
6. **重试语义**：实现 = 初始 1 + 重试 3（backoffs `{500ms,1s,2s}`），与 design D2 解读一致；docs/plan.md 注记 5 已注明（草案"3 次尝试"措辞保留供追溯）。

---

## 10. 遗留（推迟主代理 / 用户）

- **`scripts/integration-test.sh` 的 live 运行**：主代理正在并行写该脚本，本任务未触碰 `scripts/`，未跑 live 集成。
- **真实 LoRA 目录性能计时**（vet 大文件 / sync 50 文件首跑+二次）：需用户路径，主代理后续处理；条件已在 §1 项 1/2 注明（二次 ≤ 5s 依赖 hashcache 命中 + CDN 5min HIT，计时须开缓存）。
- **代码内陈旧文案修正**（§9 项 2/3）：`internal/cmd/search.go` --sort help、`internal/cmd/globals.go` login hint——本任务禁改 internal/*，提请主代理。
- **actionlint 工作流校验**：本机未装，已以人工语法审查替代（§8）；如需机器校验由主代理/CI 补。
- **最终 commit / 建仓推送 / 首次 tag 发布**：外部可见动作，需用户确认；本任务未执行任何 git commit/push/merge，未跑 `make build`（直接用现有 exe）。

---

## 11. 本任务交付物清单

- `README.md`（英文，简介→Quick start→Features→Install 三选项→Commands 全示例+全局 flag 表+D6→Exit codes→Token & config→Environment variables→Hash cache→Building from source→Reproducible builds & verifying downloads→API notes→License）
- `docs/plan.md`（中文规划原文迁移 + 头部"实现状态注记"10 条偏差决策）
- `.trellis/spec/backend/directory-structure.md`、`error-handling.md`、`quality-guidelines.md`、`logging-guidelines.md`（英文，从 design.md + 实际代码提炼）；`index.md` 状态更新；`database-guidelines.md` 标注 N/A（无数据库，持久化为 JSON 文件）
- `.trellis/tasks/11-22-civ-m5-docs/research/acceptance.md`（本文件）