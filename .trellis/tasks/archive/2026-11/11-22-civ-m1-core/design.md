# M1 技术设计 — 骨架 + API client + search/show/tags

> 全局契约见父任务 design.md（../11-22-civitai-cli-go/design.md），本文只补 M1 细节。

## 1. main.go dispatch 与全局 flag

```go
func main() {
    log.SetOutput(os.Stderr)
    log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
    if len(os.Args) < 2 { usage(); os.Exit(1) }
    switch os.Args[1] {
    case "search": cmd.Search(os.Args[2:])
    case "show":   cmd.Show(os.Args[2:])
    case "tags":   cmd.Tags(os.Args[2:])
    case "version", "-v", "--version": fmt.Println("civ", app.Version)
    case "help", "-h", "--help": usage()
    default: fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1]); usage(); os.Exit(1)
    }
}
```

- 每个 cmd.Xxx(args) 内部：flag.NewFlagSet → registerGlobalFlags(fs, &globals) → fs.Parse(args) → 构造 api.NewClient(...) → 执行 → 错误经 exitCodeFor(err) 映射 os.Exit
- globals struct{JSON bool; Token string; Timeout time.Duration; Verbose bool}；--verbose 时 log 级别放开（默认 log 只输出 [WARN]+，verbose 输出 [INFO]/[DEBUG]——用简单包级开关或 log.SetOutput(io.Discard) 切换）
- token 解析（M1 版）：globals.Token 非空用之，否则 os.Getenv("CIVITAI_API_KEY")；M3 插入 config 文件层
- 后续里程碑在 switch 增 case（vet/sync/login/logout/upgrade），main 保持仅路由

## 2. api.Client 结构

```go
type Client struct {
    httpClient *http.Client        // 默认 Timeout 30s（--timeout 覆盖）
    baseURL    string              // 默认 https://civitai.com/api/v1
    token      string
    userAgent  string              // app.UserAgent() + repo URL
    sem        chan struct{}       // 并发闸，容量 = concurrency
    backoffs   []time.Duration     // {500ms, 1s, 2s}；测试同包改短
    jitter     float64             // 0.2；测试可置 0
}
func (c *Client) Get(ctx context.Context, path string, q url.Values) ([]byte, error)
```

- Get：sem 获取（select ctx.Done）→ url = baseURL + path + "?" + q.Encode()（q 为 nil 不带 ?）→ doWithRetry → 释放 sem
- doOnce：http.NewRequestWithContext → UA/Bearer 头 → Do → io.LimitReader(body, 5MB+1) → fetchOutcome；200 且 len(body)==0 → 可重试（静默空响应坑）
- 错误解析（errors.go）：非 2xx 先试 `{"error":"..."}` 字符串形态 → 按 status 分类 NotFound/Unauthorized/ApiError；再试 `{"error":{"name":"ZodError","message":"[...]"}}` → 从 message 内嵌 JSON 数组提取首条的 message/values 拼摘要；两者都不是 → 截 300 字节原文
- 端点方法（models.go/versions.go/tags.go）：只拼 url.Values + 调 Get + json.Unmarshal 到 types；返回 ([]byte, error) 与解码后结构两种形态——search --json 需要原始 body 透传，故 Get 返回 body，端点方法提供 `SearchModels(ctx, params) (raw []byte, items []Model, err error)` 双返回（或 cmd 层自行 Unmarshal，端点只返回 raw + 解码 helper）。决策：端点方法返回 raw body，解码由 cmd 层调 types  helper（api.ParseModels(raw)），--json 直接写 raw，人类输出走解码——避免二次序列化漂移

## 3. 枚举映射与白名单（api/enums.go）

```go
var sortMap = map[string]string{"hot": "Hot", "highest-rated": "Highest Rated", "most-downloaded": "Most Downloaded", ...}
var periodMap = map[string]string{"day": "Day", ..., "all": "AllTime"}
var typeMap = map[string]string{"lora": "LORA", "checkpoint": "Checkpoint", "locon": "LoCon", "ti": "TextualInversion", ...}
var baseModelWhitelist = map[string]bool{...} // live 探测 Zod values 钉表，注释 verified live 日期
```

- 映射失败（未知 sort/type/period）→ exit 1 用法错误（列出合法值）；未知 base → stderr [WARN] 后继续
- 白名单探测：M1 实现时 `curl ".../models?baseModel=__invalid__&limit=1"` 取 Zod values 数组（附录 B 机制），连同 sort/type 合法值一并钉表

## 4. types.go 关键结构

```go
type Model struct {
    ID int; Name string; Type string; NSFW bool; NSFWLevel json.Number
    Tags []string; BaseModels []string
    AllowNoCredit bool; AllowCommercialUse []string; AllowDerivatives bool; AllowDifferentLicense bool
    Stats Stats; Creator Creator; ModelVersions []ModelVersion
    // 未知字段忽略
}
type ModelVersion struct {
    ID int; Name string; BaseModel string; PublishedAt string
    TrainedWords []string; Stats Stats; Files []ModelFile; Images []Image
    Model *ModelSummary // by-hash/版本详情带所属模型摘要
}
type ModelFile struct {
    Name string; SizeKB float64; Type string; Primary bool
    PickleScanResult string; VirusScanResult string
    Metadata FileMetadata; Hashes map[string]string // SHA256/AutoV2/... 全量保留
}
type Image struct {
    URL string; NSFWLevel json.Number; HasMeta bool; Meta *ImageMeta // null 安全
}
```

- 渲染 helper：NSFWLevelText(json.Number) 按位拼接（1 None/2 Soft/4 Mature/8 X/未知 L<bit>）；RatioText(up, down) "178:1"；CompactNumber(45200)="45.2k"
- 日期：publishedAt/updatedAt ISO8601 → 截取前 10 位 YYYY-MM-DD（解析失败显示原文）

## 5. report 渲染

- table.go：tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)；列宽预算按终端宽度（consoleWidth() 失败 80）；NAME 列截断（预算内最长，超出加 …）
- search 表格列：ID/NAME/TYPE/BASE/DL/RATIO/UPDATED/NSFW/CREATOR（规划文档 §4.1 样例）
- show 模型头：键值对齐块 + 许可四件套一行；版本表：ID/NAME/BASE/PUBLISHED/DL/RATIO/TRIGGERS(n)
- 触发词块：每行一条原样输出（无引号无符号）
- json.go：--json 时直接 os.Stdout.Write(raw)（search 透传）或 json.MarshalIndent 结构化对象（show/tags）

## 6. fixture 与测试布局

- testdata/*.json 附录 D 抓取；测试用 os.ReadFile("../testdata/...") 相对路径（cmd 包测试在 internal/cmd/ 下，用 ../../testdata）——统一 helper testutil.LoadFixture(t, name) 放 internal/testutil（仅测试引用，非导出业务）
- client_test.go：countingServer（atomic 计数）验证重试次数；newFastTestClient 同包改 backoffs={0,0,0} jitter=0；httptest 覆盖 UA/Bearer/空 body 重试/429+Retry-After/5xx/404 不重试/并发闸上限
- types_test.go：表驱动解码（fixture + 内联畸形 JSON：nsfwLevel 字符串混入 models 端点、meta null、stats {}、未知枚举）
- cmd 测试：mock 端点（httptest 返回 fixture）断言表格含关键行、--json 可解析、退出码映射

## 7. Makefile（M1 基础版）

```make
BINARY := civ
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')
ifeq ($(VERSION),)
VERSION := 0.1.0-dev
endif
GOARCH ?= $(shell go env GOARCH)
LDFLAGS := -s -w -X civitai-cli/internal/app.Version=$(VERSION)
build: dist/$(GOARCH)/$(BINARY).exe   # GOOS=windows CGO_ENABLED=0 -trimpath -buildvcs=false
```

M4 补 build-dist/zip/installer/release/dist-all/integration/test-race 已有则保留。注意：本仓库尚未 git init，git describe 失败走兜底——M1 期间先 git init（本地，无 remote，不推送）。