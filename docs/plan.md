# Civitai CLI（`civ`）规划文档

> **实现状态注记（M5，2026-11-22 追加）**
>
> 本文是 v0.1 规划草案的原文迁移（保留中文，作为 API 事实规范与设计决策的历史
> 事实源）。v1 已按 M1–M5 全部实现并接线验证：`search` / `show` / `vet` /
> `sync` / `tags` / `login` / `logout` / `upgrade` / `version` / `help` 全部可用，
> 二进制 `dist/amd64/civ.exe` 已构建。**面向用户的英文文档见仓库根 `README.md`；
> 本文件保留规划原文，下方注记列出实现与草案的有意偏差。**
>
> **v1 已实现范围**：规划 §4 的六命令 + 自更新/版本/帮助；零第三方依赖单文件
> （`CGO_ENABLED=0`）；匿名默认 + 可选 token；`--json` 全命令契约；退出码
> 0/1/2/3/4；sync 哈希缓存（size+mtime 失效）；可复现构建 + NSIS/install.ps1
> 分发 + CI/release。非目标（§1）全部遵守：不下载、不抓图、不上传、不解析登记
> 册、不绕过 NSFW。
>
> **偏差决策（实现以此为准，原文保留供追溯）**：
>
> 1. **D6 — 全局 `-v` 冲突**：草案 §4 的全局 `-v`（调试）与 `show -v <versionId>`
>    冲突。实现决策：全局调试 flag 为 **`--verbose`（无短别名）**；`show` 的
>    `-v` / `--ver` 保留版本语义。`-v` / `--version` 仅作 `civ version` 的别名
>    （无命令上下文，不冲突）。README 已注明。
> 2. **省略 `api/images.go`**：v1 无命令消费 `/images` 浏览端点；`--prompts` 走
>    **版本详情的 `images[].meta`**（§4.2 功能定义为准）。草案 §7 目录里的
>    `images.go` 未建。匿名 NSFW 钳制只装在 `/images` 浏览端点，版本详情匿名全量
>    返回，故 vet/show 不受影响。
> 3. **`hashcache` 兼放流式哈希计算**：`ComputeSHA256`（M2）与缓存读写（M3）同处
>    `internal/hashcache`，未为单一函数另立包。`vet` 每次新鲜计算（不走缓存）；
>    缓存仅 `sync` 使用。
> 4. **`baseModel` 单数参数已死 → 改用复数 `baseModels`**：实测单数 `baseModel`
>    对**所有**值静默忽略；生效的是复数 `baseModels`，且**仅首值生效**（重复参数
>    首值胜出，逗号连接被当作一个未知值 → 空结果）。CLI 因此只送首个 `--base`
>    值并对多值告警；未知值对照 63 项白名单告警后放行（API 返回空而非报错）。
>    草案 §3.3-2 / 附录 A.1 的"逗号多值"描述以本注记为准。
> 5. **重试语义 = 初始 1 次 + 重试 3 次**：草案 §6-D2 的"3 次尝试 + 500ms→1s→2s"
>    解读为初始 1 次 + 重试 3 次（退避序列恰 3 项 `{500ms, 1s, 2s}` + ±20% 抖动，
>    封顶 8s，尊重 `Retry-After`）。
> 6. **新增基建包** `internal/{app,config,env,upgrade}`：app/env/upgrade 来自参考
>    项目（用户追加的自更新/PATH 自举需求）；config 独立成包避免 cmd/login 与 api
>    纠缠。草案 §7 目录未含这些包。
> 7. **`types` 多值用重复 `types=` 参数**：逗号连接 `types=LORA,LoCon` 是 ZodError；
>    重复 `types=LORA&types=LoCon` 生效。
> 8. **`sort=Recently Added` 需 `collectionId`**：plain `/models` 搜索用它返回
>    HTTP 400（第三种错误形态 `{"message":"..."}`）；枚举表保留该值（Zod values
>    列出），但普通搜索不可用。
> 9. **Go 版本钉死**：草案 §2 写"1.24+"；实现钉死 `go 1.27` / `toolchain go1.27.3`
>    （可复现构建要求同一编译器补丁版本，CI `go-version-file` 读取）。
> 10. **退出码细化**：`404` 在 `search`/`show`/`tags` 归用法类 `1`，仅 `vet` 归 `3`；
>     `sync` 的 `not_found` 是数据行（整体 exit `0`）；匿名 `401`（个人态参数）归
>     `1` 并提示需 token，带 token 的 `401` 才归 `4`。
>
> 验收逐条核对见 `.trellis/tasks/11-22-civ-m5-docs/research/acceptance.md`。
> 全局技术设计契约见 `.trellis/tasks/11-22-civitai-cli-go/design.md`（D1–D9）。

---

> 状态：草案 v0.1（2026-11-22）
> 调研基础：2026-11-22 对线上 API 的实测（非文档转述；官方无 OpenAPI 规范，本文附录 A 即事实规范）
> 用途：新仓库启动蓝图；建仓后建议挪至 `docs/plan.md` 或 `PLAN.md`

---

## 1. 定位

Civitai 模型搜索与选卡审查的命令行工具。解决两件事：**找卡**（搜索/筛选/对比）与**审卡**（本地文件反查元数据、触发词、许可、安全扫描、例图配方）。

### 目标

- 秒级完成"这个 LoRA 是谁、什么底模、触发词是什么、安不安全、别人怎么用"
- 批量对账本地 LoRA 目录与 Civitai 登记信息（触发词回填登记册）
- 零配置可用（匿名），可选 token 解锁个人态功能
- 单一静态二进制，无运行时依赖，Windows 优先

### 非目标（v1 明确不做）

- **不做模型下载**。下载交给浏览器/aria2/ComfyUI 生态，二期再议（`civ dl` 候选）
- 不做图片下载与浏览（vet 报告只给例图 URL，不抓图）
- 不做上传/创作者写操作
- 不解析任何本地登记册格式（`sync` 输出 CSV/JSON，对账由人或其他工具完成）
- 不绕过 NSFW 门控（匿名就是匿名；带 token 也只显示账号设置允许的内容）

## 2. 命名与分发

| 项 | 建议 |
|---|---|
| 二进制名 | `civ` |
| 仓库名 | 自定（建议 `civitai-cli`） |
| Go 版本 | 1.24+ |
| 依赖策略 | **零运行时依赖**（仅标准库），理由见 §6-D1 |
| 构建 | `CGO_ENABLED=0 go build`，单文件；release 用 goreleaser 或手工 `GOOS/GOARCH` 交叉编译 |

## 3. API 事实层（2026-11-22 实测）

### 3.1 基础

- Base URL：`https://civitai.com/api/v1`；读取面全匿名可用，token 可选（`Authorization: Bearer <key>`）
- 分页：`limit` 硬上限 **100**；游标式 `cursor`（响应 `metadata.nextCursor`/`nextPage`）
- 缓存：CDN `s-maxage=300`（相同 URL 5 分钟内免费 HIT）
- 延迟：origin 响应 1.3–2.0s；实测匿名 20 并发全 200，无 ratelimit 响应头，无官方限流数字
- 错误两种形态要兼容：参数校验失败 `{"error":{"name":"ZodError",...}}`（400）；资源不存在 `{"error":"Model not found"}`（404）

### 3.2 端点（★ = vetting 核心）

| 端点 | 用途 |
|---|---|
| `GET /models` | 搜索/列表 |
| `GET /models/{id}` | 模型详情（与搜索 item 同构） |
| `GET /model-versions/{id}` | 版本详情：files + images（含生成参数 meta） |
| ★ `GET /model-versions/by-hash/{hash}` | 哈希反查版本。SHA256 全串、AutoV2（= SHA256 前 10 位十六进制大写）均可；未知哈希 404 |
| `GET /images` | 例图浏览（挖 prompt 用；匿名 NSFW 钳到 Soft） |
| `GET /tags?query=` | tag 补全 |
| `GET /creators?query=` | 创作者查询 |
| `GET /api/download/models/{versionId}` | 下载跳转（v1 不实现） |

### 3.3 已验证的坑（实现时必须处理）

1. **枚举值含空格，必须 URL encode**：`sort=Most%20Downloaded`，裸空格导致静默空响应（不发错误、curl 退出码 0、body 空——按可重试错误处理）
2. **`baseModel` 过滤只认平台登记值**：新底模（如 `Anima`）被**静默忽略**（不报错、返回无关结果）。CLI 侧应白名单校验并警告
3. **`nsfwLevel` schema 不一致**：models 端点是数字位掩码，images 端点是字符串（`"None"`/`"Soft"`/…），解码层分开处理
4. **NSFW 门控只装在 `/images` 浏览端点**：匿名默认只回 SFW，`nsfw=X|Mature` 被钳到 Soft 且不报错；**但版本详情的 `images[]` 匿名全量返回**（含 Mature 例图及其 prompt meta）——vet 流程走版本详情不受影响
5. **401 仅限个人态参数**：`favorites=true`、`hidden=true` 匿名报 `{"error":"Unauthorized"}`；其余读取面无 401
6. 旧文档的 `/notifications`、`/buzz` 已 404（个人面 API 迁至新版 Site API，v1 不碰）

### 3.4 token 的实际差距（针对"到底带不带"）

token 是身份证不是 VIP 卡：读取能力零提升，只多三样——
① `favorites`/`hidden` 搜索过滤；② `/images` 端点看 Mature/X（取决于账号 NSFW 设置）；③ 下载门控资源（付费/early access/私有）。
**结论：默认匿名，token 做成可选增强。**

## 4. 命令设计

全局 flag（所有子命令）：`--json`（机器可读输出）、`--token`（覆盖环境变量）、`--timeout 30s`、`-v`（调试日志走 stderr）。

### 4.1 `civ search <query>` — 找卡

```
civ search stocking --type lora --base Illustrious --sort most-downloaded --period month --limit 20
```

| flag | 说明 |
|---|---|
| `--type` | lora / checkpoint / locon / ti / hypernet / controlnet / vae / upscaler / poses / wildcards / other（大小写不敏感，映射到 API 的 LORA 等值；可重复或逗号分隔） |
| `--base` | 逗号分隔；**非登记值给警告**（参考 §3.3-2） |
| `--sort` | hot（默认）/ highest-rated / most-downloaded / most-liked / most-discussed / most-collected / most-images / newest / oldest / recently-added |
| `--period` | day / week / month / year / all（默认 all） |
| `--tag` `--user` | 精确 tag / 创作者过滤 |
| `--limit` | ≤100，默认 20 |

默认输出（表格，80 列内不折行失控，长名截断）：

```
ID       NAME                          TYPE    BASE          DL      RATIO    UPDATED    NSFW  CREATOR
182296   Pantyhose & Stockings NSFW    LORA    SD1.5         45.2k   178:1    2026-03-12 Yes   creator_name
```

`--json`：原始 items 数组透传（字段以 API 为准，不做裁剪，便于脚本消费）。

### 4.2 `civ show <modelId>` — 看卡

```
civ show 182296                     # 模型头 + 版本表
civ show 182296 -v 482391           # 钻进指定版本：文件表 + 哈希 + 触发词
civ show 182296 -v 482391 --prompts # 追加例图配方（prompt/negative/steps/cfg/sampler）
```

- 模型头：名称、创作者、类型、标签、许可四件套（`allowNoCredit`/`allowCommercialUse`/`allowDerivatives`/`allowDifferentLicense`）、统计（下载/赞踩比/评论/打赏）、更新时间、CivitAI 页面 URL
- 版本表：版本 ID、名称、baseModel、发布日、下载量、赞踩比、触发词条数
- 版本详情：文件表（名称、大小、format、fp、primary、**全量哈希**）、pickle/virus 扫描结果、触发词块（纯文本每行一条，便于复制进登记册）
- `--prompts`：取该版本 `images[].meta`，按例图编号输出 prompt / negativePrompt / steps / cfgScale / sampler / scheduler（meta 为 null 的例图跳过并计数）

### 4.3 `civ vet <path|sha256|autov2>` — 审卡（旗舰命令）

输入本地文件路径（流式算 SHA256）或直接给哈希；`by-hash` 反查后输出审查报告：

```
$ civ vet "I:\ComfyUI-Workspace\models\loras\foo.safetensors"

MODEL     Foo Bar LoRA            https://civitai.com/models/xxxxx
VERSION   v3.0  (2026-09-01)      base: Illustrious
CREATOR   someone                 DL 12.3k   ratio 145:1   comments 34

TRIGGER WORDS
  foo bar, ao no trigger, ...

FILES (1)
  foo_v3.safetensors  231.4 MB  SafeTensor bf16  primary
  sha256   9F2A...E1
  autov2   9F2A11BC22
  scan     pickle: Success   virus: Success

LICENSE   credit: yes   commercial: Image   derivatives: yes   relicensing: no

EXAMPLES  5 (mature 2)   --prompts 查看例图配方
```

退出码：`0` 命中 / `3` 未收录（404，可能非 Civitai 来源或文件被改）/ `1` 用法错误 / `2` 网络或重试耗尽 / `4` token 失效。

### 4.4 `civ sync <dir>` — 批量对账

```
civ sync I:\ComfyUI-Workspace\models\loras --format csv > audit.csv
```

- 递归扫描（`--ext` 默认 `.safetensors,.pt,.pth,.ckpt,.bin`）
- 哈希缓存：`%LocalAppData%\civ\cache\hashes.json`（path + size + mtime → sha256），改动才重算；`--rehash` 全量
- API 并发默认 3（`--concurrency`，上限 8）；文件哈希并发默认 4
- 输出行：FILE | STATUS(found/not_found) | MODEL | VERSION | BASE | TRIGGERS(n) | PICKLE | VIRUS | MODEL_URL
- 同一版本命中多个本地文件时标 `dupe`

### 4.5 `civ tags <query>` — tag 补全

小命令，返回 `GET /tags` 的前 N 条名称列表。给 prompt 工程顺手用。

### 4.6 `civ login` / `civ logout` — token 管理

```
civ login     # 交互粘贴 key → 用 favorites 探针校验（GET /models?favorites=true&limit=1，200=有效，401=无效）
              # 写入 %AppData%\civ\config.json（0600 权限语义：Windows 下尽力而为）
civ logout    # 删除配置文件
```

优先级：`--token` flag > 环境变量 `CIVITAI_API_KEY` > 配置文件。

## 5. 输出约定

- 人类输出：stdout 表格（`text/tabwriter`）；调试/进度：stderr
- 触发词、prompt 配方：纯文本块，每行一条，**不加引号、不加列表符号**，可直接复制
- 所有命令支持 `--json`；`--json` 时人类装饰全关，只出结构化数据
- 金额/数量统一 k/M 缩写；日期 `YYYY-MM-DD`；时长秒级一位小数
- 终端宽度取 `stdout` 句柄 CONSOLE 信息，取不到按 80

## 6. 技术方案

### D1 零依赖（标准库-only）

| 关注点 | 方案 |
|---|---|
| CLI 子命令 | 手写 dispatch + `flag` 包（6 个命令不值得引 cobra；命令树膨胀再议） |
| 表格 | `text/tabwriter` |
| HTTP | `net/http` + 显式 `http.Client{Timeout}` |
| JSON | `encoding/json`（`Decoder.UseNumber()` 应对 nsfwLevel 数字/字符串漂移） |
| 哈希 | `crypto/sha256` 流式 |
| 配置/缓存路径 | `os.UserConfigDir()` / `os.UserCacheDir()` |
| 测试 | `net/http/httptest`  fake server，测试零外网 |

### D2 HTTP 客户端行为

- UA：`civ/0.1 (+<repo-url>)`
- 重试：3 次尝试，指数退避 500ms→1s→2s + 抖动，封顶 8s；重试条件：传输错误、**空 body**、429、5xx；不重试：400/401/403/404（401 且带 token 时提示重新 login）
- 尊重 `Retry-After`（当前不返回，成本极低顺手实现）
- 全局在飞请求上限 = `--concurrency`（默认 3）

### D3 错误分类

```
ApiError{ZodDetail}   // 400 参数类：打印 Zod message 摘要，提示 CLI 侧 bug 或 API 漂移
UnauthorizedError     // 401：token 问题，提示 login
NotFoundError         // 404：按命令语义处理（vet→退出码 3）
NetworkError          // 传输/空 body/5xx 重试耗尽
```

Zod message 原文多行 JSON，展示时截取首条 `message` 字段即可。

### D4 schema 容错原则

API 无版本承诺且错误信息会泄露枚举变化，因此：

- 结构体一律宽容解码：未知字段忽略，不认识的枚举值原样保留（string 接收 + 映射函数渲染，映射失败显示原文）
- `nsfwLevel` 两处类型不同，models 系列用 `json.Number` 位掩码 + `String()`，images 系列用 string（附录 C）
- 数值字段可能缺省（`stats` 全空对象），渲染层 nil 安全

## 7. 目录结构

```
civitai-cli/
├── go.mod
├── main.go                  // 仅 dispatch + 全局 flag
├── internal/
│   ├── api/
│   │   ├── client.go        // http.Client 封装、重试、并发闸、认证头
│   │   ├── models.go        // /models、/models/{id}
│   │   ├── versions.go      // // /model-versions/{id}、by-hash
│   │   ├── images.go        // /images（--prompts 用）
│   │   ├── tags.go          // /tags
│   │   ├── types.go         // 全部响应结构体 + 容错解码
│   │   └── errors.go        // 错误分类、两种错误形态解析
│   ├── cmd/
│   │   ├── search.go
│   │   ├── show.go
│   │   ├── vet.go
│   │   ├── sync.go
│   │   ├── tags.go
│   │   └── login.go
│   ├── report/
│   │   ├── table.go         // tabwriter 封装、截断、对齐
│   │   └── json.go
│   └── hashcache/
│       └── cache.go         // 哈希缓存读写、mtime+size 失效判定
├── testdata/                // 线上抓取的 fixture（附录 D 命令可再生成）
└── docs/plan.md             // 本文
```

> GoDoc 规范：所有导出符号必须有文档注释，遵循官方 `// Symbol ...` 惯例（与作者全局 AGENTS.md 注释规则一致）。

## 8. 里程碑

| 里程碑 | 内容 | 验收 |
|---|---|---|
| M1 | api client + `search` + `show` | 能对 anima/stocking 出表，`--json` 可解析 |
| M2 | `vet` | 本地任意 LoRA 反查出报告；404 退出码 3 |
| M3 | `sync` + `login/logout` + `--prompts` | 50 文件目录对账 ≤ 40s；缓存命中二次 ≤ 5s |
| M4 | 测试、fixture、README、release 产物 | 测试零外网；README 含全部命令示例 |

每日可用：M1 起每个里程碑都是完整可用工具。

## 9. 风险与对策

| 风险 | 对策 |
|---|---|
| API 无规范、字段漂移 | §6-D4 宽容解码；附录 D fixture 定期重抓比对 |
| 静默空响应（裸空格类畸形 URL） | 客户端统一经 `url.Values` 编码构造 URL，杜绝手拼 |
| 限流形态未知 | 空 body/5xx/429 全部走退避重试；并发默认 3，远低于实测 20 并发全过的水位 |
| CDN 5 分钟缓存导致"刚发的模型查不到" | 文档注明；`--json` 输出附响应时间戳 |
| 匿名 NSFW 钳制 | vet/show 走版本详情不受影响；`--prompts` 注明匿名上限 Soft |
| 合规 | 遵守站点 ToS；不抓图、不下载、礼貌并发 |

## 10. 验收标准（汇总）

- `vet` 单文件 < 3s（缓存命中 < 1s）
- `sync` 50 文件 ≤ 40s，二次运行 ≤ 5s
- 全部命令支持 `--json`，输出契约稳定
- 测试不访问外网（httptest + fixture）
- 退出码语义文档化且稳定
- `go build` 单文件二进制，无 CGO

---

## 附录 A：端点与参数事实规范（2026-11-22 实测）

### A.1 `GET /api/v1/models`

| 参数 | 值 | 备注 |
|---|---|---|
| `query` | 自由文本 | |
| `type` / `types` | Checkpoint, LORA, LoCon, DoRA, TextualInversion, Hypernetwork, AestheticGradient, Controlnet, Poses, Wildcards, Upscaler, VAE, MotionModule, Other | 单复数皆可 |
| `baseModel` | 逗号多值 | **仅平台登记值生效，未知值静默忽略**（实测 `Anima` 被忽略） |
| `tag` | 单 tag 名 | 精确匹配 |
| `username` | 创作者名 | |
| `sort` | Hot / Highest Rated / Most Downloaded / Most Liked / Most Discussed / Most Collected / Most Images / Newest / Oldest / Recently Added | **含空格，必须 URL encode** |
| `period` | Day / Week / Month / Year / AllTime | |
| `favorites` `hidden` | bool | 匿名 401，需 token |
| `limit` | 1–100，默认 100 | >100 报 Zod `too_big` |
| `cursor` | `metadata.nextCursor` | 旧式 `page` 仍可用 |

响应 item 关键字段（vetting 相关）：`id, name, type, nsfw(bool), nsfwLevel(数字), description(HTML), tags[], baseModels[], allowNoCredit, allowCommercialUse[], allowDerivatives, allowDifferentLicense, availability, publishedAt?（在版本上）, stats{downloadCount, thumbsUpCount, thumbsDownCount, commentCount, tippedAmountCount}, creator{username}, modelVersions[]`

版本对象：`id, name, baseModel, baseModelType, publishedAt, trainedWords[], availability, nsfwLevel(数字), stats, downloadUrl, files[], images[]`

文件对象：`name, sizeKB, type(Model|VAE|...), primary, pickleScanResult, virusScanResult, scannedAt, metadata{format, fp, size}, hashes{SHA256, AutoV1, AutoV2, AutoV3, BLAKE3, CRC32, SHA256_12}, downloadUrl`

图片对象（版本内）：`url, nsfwLevel(数字), hasMeta, meta{prompt, negativePrompt, steps, cfgScale, sampler, scheduler, seed, width, height, models[], resources[], ...}`（**meta 可能为 null**；`hasMeta=true` 不保证 meta 非空）

### A.2 `GET /api/v1/models/{id}`

与搜索 item 同构，含全部版本。

### A.3 `GET /api/v1/model-versions/{id}`

单版本完整对象（同 A.1 版本字段），`model` 字段含所属模型摘要（`id, name, type`）。

### A.4 `GET /api/v1/model-versions/by-hash/{hash}`

- 接受：SHA256 64 位十六进制（大小写不敏感）、AutoV2 10 位（= SHA256 前 10 位大写）
- 未命中：404 `{"error":"Model not found"}`

### A.5 `GET /api/v1/images`

| 参数 | 值 | 备注 |
|---|---|---|
| `modelVersionId` / `modelId` | id | |
| `sort` | Most Reactions / Newest / Oldest / Random / ... | 含空格，encode |
| `period` | Day / Week / Month / Year / AllTime | |
| `nsfw` | X / Mature / Soft 等 | **匿名被钳到 Soft 且不报错**；默认只回 SFW |
| `limit` `cursor` | 同 A.1 | |

item 额外字段：`browsingLevel(数字), stats{likeCount,...}, postId, meta`（**此处 nsfwLevel 是字符串**）

### A.6 `GET /api/v1/tags`、`GET /api/v1/creators`

均支持 `query` 前缀过滤 + `limit`/`page`。tags 返回 `{name, link}`；creators 返回 `{username, modelCount, image, link}`。

## 附录 B：错误形态样例

```json
// 400 参数错（ZodError；values 数组会泄露当前合法枚举）
{"error":{"name":"ZodError","message":"[\n  {\n    \"code\": \"invalid_value\",\n    \"values\": [\"Hot\",\"Highest Rated\", ...]\n  }\n]"}}

// 404
{"error":"Model not found"}

// 401（匿名调用个人态参数）
{"error":"Unauthorized"}

// limit 超界（同样 ZodError 形态）
{"error":{"name":"ZodError","message":"[{\"origin\":\"number\",\"code\":\"too_big\",\"maximum\":100, ...}]"}}
```

解析策略：见 `{"error":"..."}` 取字符串；见 `{"error":{"name":"ZodError"...}}` 提取 `message` 内首个 JSON 片段的 `message`/`values` 字段。

## 附录 C：nsfwLevel 实测归纳

| 场景 | 形态 | 实测值 |
|---|---|---|
| models 系列端点 | 数字位掩码 | 模型 7/31（多级别叠加），版本 3，图片 2/4 |
| images 浏览端点 | 字符串 | `"None"`、`"Soft"`；另有 `browsingLevel` 数字（Soft=2） |

数字掩码归纳（由观测值反推，未知位容错渲染）：

```
1 = None    2 = Soft    4 = Mature    8 = X    16+ = 未知
渲染：按位取已知名拼接，未知位显示 "L<bit>"
```

images 端点字符串与数字掩码并存，解码层独立处理，不做互相推断。

## 附录 D：fixture 再生成命令

测试 fixture 从线上抓取后入 `testdata/`，以下命令可再生成（抓后脱敏：无需脱敏，全部公开数据）：

```bash
# 模型搜索
curl -s "https://civitai.com/api/v1/models?limit=2&query=anima" > testdata/models_search_anima.json
# 模型详情
curl -s "https://civitai.com/api/v1/models/2458426" > testdata/models_2458426.json
# 版本详情（含 meta 的例图）
curl -s "https://civitai.com/api/v1/model-versions/3263843" > testdata/version_3263843.json
# 哈希反查（Anima turbo-v1.1 的 SHA256）
curl -s "https://civitai.com/api/v1/model-versions/by-hash/FBA11953276B57EDF59D1DC4F1857AC05AA079C56F982B4D7C20298D57D3F7EB" > testdata/byhash_anima_turbo.json
# 错误形态
curl -s "https://civitai.com/api/v1/models?limit=200" > testdata/err_limit_too_big.json
curl -s "https://civitai.com/api/v1/model-versions/by-hash/DEADBEEF" > testdata/err_not_found.json
curl -s "https://civitai.com/api/v1/models?limit=1&favorites=true" > testdata/err_unauthorized.json
```

CI 中绝不访问外网；fixture 漂移时手工重抓并人工核对字段增减。