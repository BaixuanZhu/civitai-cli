# danbooru-tag-mcp 代码组织研究报告

> 来源：Explore 代理对 I:\GitDownload\danbooru-tag-mcp 的只读研究，2026-11-22。
> 用途：civitai-cli 目录结构、编码约定、测试模式的设计依据。

## 1. 顶层结构

- go.mod：module 名为裸名 `danbooru-tag-mcp`（无域名前缀）；`go 1.26` + `toolchain go1.26.5` 精确钉死补丁版本（可复现构建要求同一编译器，CI 用 go-version-file 读取，升级须与 CI 同步）。仅 1 个第三方依赖 `github.com/mark3labs/mcp-go v1.1.1`（MCP 协议库），业务代码全标准库。civitai CLI 无协议需求，可真正零依赖。
- go.work：仅 `go 1.26` / `use .`——单 module，作用统一钉住工作区 Go 版本。
- main.go 只做路由（手写 `switch os.Args[1]`，不用 cobra/flag——其子命令无参数）：无参 = MCP server 模式；`upgrade` → upgrade.Run()；`version/-v/--version` → 打印 app.Version；`help/-h/--help` → usage()；未知命令 → stderr + usage + exit 1。main 里 `log.SetOutput(os.Stderr)` + `Ldate|Ltime|Lshortfile`；启动时 `if app.BootstrapEnabled() { env.EnsureUserPath() }` + `upgrade.CleanupStaleBak()`。
- 其他顶层：Makefile、install.ps1、installer/*.nsi、scripts/zipfile、scripts/integration-test.sh、CHANGELOG.md、.github/workflows（ci.yml + release.yml）。

## 2. internal/ 包职责与依赖方向

依赖方向：`main.go（仅路由）→ tools → service → api`，层间用**消费方定义的接口**解耦。

| 包 | 职责 |
|---|---|
| api/ | REST HTTP client；`Client.Get` 是所有出站请求的**唯一咽喉点**（限流/重试/UA/凭证/body 上限/错误收缩） |
| app/ | 跨包共享基础设施：Version、Fail、UserAgent、bootstrap 开关（集中放置避免 import cycle） |
| service/ | 业务逻辑 + JSON 解析；依赖 `TagFetcher` 接口，不认识具体 client |
| tools/ | MCP 工具注册与参数适配（对应 CLI 的命令 handler 层） |
| env/ | Windows 用户 PATH 注入（HKCU 注册表 + WM_SETTINGCHANGE 广播，不用 setx） |
| vocab/ | `//go:embed data/*.csv` 词表叶子包 |
| cache/ | 标准库-only 持久响应缓存叶子包（sha256 键 JSON 文件、原子写、双上限淘汰） |
| upgrade/ | GitHub Release 自更新（版本比较→下载→SHA256 校验→.bak 原子替换） |

api 包核心（functional options 模式）：

```go
type Client struct {
    httpClient *http.Client
    baseURL    string
    userAgent  string
    reqGap     time.Duration
    lastReq    time.Time      // mu 只保护限流记账；sleep/网络/缓存写全部不持锁
    mu         sync.Mutex
    backoffs   []time.Duration // 第 N 次重试前等待；测试里直接改短
}
type Option func(*Client)
func WithBaseURL(u string) Option
func WithReqGap(d time.Duration) Option
func WithHTTPClient(httpClient *http.Client) Option
func NewClient(opts ...Option) *Client  // httpClient 默认 Timeout 30s；backoffs {1s,3s}

// 单次尝试的原始结果，重试分类前的中间结构
type fetchOutcome struct {
    body []byte; status int; contentType, retryAfter string
    err error; fatal bool // fatal = 重试无法修复（请求构建失败、超 5MB）
}
// 可重试错误标记类型：5xx/网络错/429，errors.As 区分
type transientError struct{ err error }  // 实现 Unwrap
```

要点：超时挂 http.Client；重试 ctx 可中断（`select ctx.Done()/time.After`）；429 尊重 Retry-After（秒数或 HTTP-date，cap 30s）；错误体收缩——HTML 提取 `<title>`，其他截 300 字节；`io.LimitReader(resp.Body, 5MB+1)` 检测溢出 → fatal；4xx 是确定性答案不重试。

types 组织：api 包**不放响应 struct**，只返回原始 body；JSON 类型集中在消费方（service/tag.go），每字段 json tag + 行尾注释解释语义（"verified live"）。端点方法只是拼 url.Values 后调 Get。接口定义在消费方（service.TagFetcher、tools.TagService）。

## 3. 测试模式

- **没有 testdata 目录，fixture 全部内联**在 _test.go（civitai-cli 按规划文档改用 testdata/ 文件 fixture，附录 D 可再生成）。
- api 层 = httptest fake server：helper `newTestClient(srv.URL)` = WithBaseURL + WithReqGap(0)；`newFastTestClient` 同包直改私有字段 `cli.backoffs = []time.Duration{0,0}`（重试测试秒级跑完，不加测试专用导出 API）；`countingServer` 用 atomic.AddInt32 计数验证重试次数。
- service 层 = mock struct：canned 响应 body + 调用参数记录（断言发出去的请求）+ 错误注入三类字段。
- 真实数据 fixture：`const wikiBodyArmUp = \`...\`` 钉住真实线上正文，注释要求"上游变更时有意更新 fixture"。
- 分层：单元 = `go test ./...`（httptest + mock，零网络，无 build tag）；集成 = scripts/integration-test.sh（真实 exe + 真实外网 + python 断言），强制 NO_CACHE=1 NO_BOOTSTRAP=1 隔离，CI 仅每周 schedule/手动，定位"上游 API 漂移金丝雀"。
- 命名约定 `TestXxx_Behavior`；注册表类测试需 `DANBOORU_MCP_TEST_REGISTRY=1` 才跑（**civitai-cli 对应 CIV_TEST_REGISTRY=1，且只在 CI 隔离 VM 执行，禁止本机跑**——用户既定规则）。

## 4. AGENTS.md 编码约定（参考项目）

- **全英文**：注释、GoDoc、CLI 输出、日志、测试名、脚本 echo 一律英文（zh-CN Windows 控制台中文乱码曾坑过 make）。
- 分层铁律：所有 Go 代码在 internal/；新端点必须走 Client.Get 唯一咽喉点，禁止直接用 http；接口在消费方定义。
- 产品边界写成"test-enforced, do not break"段落：每条边界指名钉住它的测试，记录决策历史与"verified live + 日期"实证结论——注释解释 why 而非复述代码。
- stdout 纪律：日志/进度全 stderr；stdout 只放命令结果（人类表格或 --json 数据）。
- 可复现构建：toolchain 精确钉死、CGO_ENABLED=0 -trimpath -buildvcs=false、自写确定性 zip（禁 Compress-Archive）、CI verify-repro 重 build 比对哈希。
- Windows + Git Bash（MSYS_NO_PATHCONV=1）。

## 5. README.md 结构（参考项目）

`# 名称`（简介）→ `## Quick start` → `## Features` → `## Install`（setup wizard / one-line PowerShell / portable zip 三选项）→ 配置 → 工具/命令表 → 缓存说明 → CLI → `## Environment variables`（表）→ `## Building from source` → `## Reproducible builds & verifying downloads`。

## 6. .gitattributes

`* text=auto eol=lf` 强制 LF（Windows autocrlf 造成 phantom modified + Git Bash/CI 下 `$'\r': command not found`）；go/md/json/yml/yaml/sh/nsi/ps1/Makefile 显式 `text eol=lf`；exe/zip `binary`；`.trellis/workspace/*/journal-*.md merge=union`。

## 7. 代码风格

- 错误处理：默认 `fmt.Errorf("...: %w", err)` 包装带包前缀；仅在需要**分类**时定义自定义类型（transientError 可重试标记实现 Unwrap、NotFoundError 业务语义）；sentinel 用 errors.New。
- 日志：标准库 log，main 里 SetOutput(os.Stderr) + Ldate|Ltime|Lshortfile；级别手写前缀 [INFO]/[WARN]/[FATAL]；致命 app.Fail（stderr+exit 1）；降级不致命（缓存坏 → WARN 后继续）。
- 导出符号：每个都有以符号名开头的完整 GoDoc，常附设计理由/并发契约/实证来源；包注释 `// Package xxx ...`；私有函数也普遍有注释；常量集中定义并注释取值依据。
- context：显式 ctx 首参贯穿 handler→api→http.NewRequestWithContext；所有等待 select ctx.Done()。