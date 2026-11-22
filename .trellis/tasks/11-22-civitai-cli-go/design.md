# 总体技术设计 — civitai-cli (civ)

> 需求见 prd.md；API 事实规范见 research/civitai-cli-plan.md 附录 A-D；参考项目机制见 research/danbooru-*.md。
> 本文是全局设计契约，子任务 design.md 只补充各自细节，冲突时以本文为准并回改。

## 1. 架构与目录

依赖方向（单向，接口在消费方定义）：

```
main.go（仅 dispatch + 全局 flag 装配）
  └─ internal/cmd/*（命令 handler：flag 解析 → 调 api → 调 report 渲染 → 退出码）
       ├─ internal/api（唯一出站咽喉点 Client.Get + 端点方法 + 容错 types + 错误分类）
       ├─ internal/report（tabwriter 表格 / JSON / k-M 缩写 / 日期 / 截断 / 终端宽度）
       ├─ internal/hashcache（流式 SHA256 + path/size/mtime 缓存）
       └─ internal/config（token 读写与优先级解析）
  基础设施（M4）：internal/app（Version/Fail/UserAgent/Bootstrap）、internal/env（PATH 自举）、internal/upgrade（自更新）
```

```
civitai-cli/
├── go.mod                  # module civitai-cli; go 1.27; toolchain go1.27.3（本机版本，CI go-version-file 读取）
├── main.go                 # dispatch + 全局 flag；启动时 CleanupStaleBak + Bootstrap（M4 接线）
├── Makefile                # M1 基础 target；M4 补全 dist/installer/release
├── install.ps1             # M4，全 ASCII 无 BOM
├── installer/civitai-cli.nsi
├── scripts/zipfile/main.go # 确定性 zip（原样移植）
├── scripts/integration-test.sh
├── .github/workflows/{ci,release}.yml
├── .gitattributes          # * text=auto eol=lf；exe/zip binary；journal merge=union
├── CHANGELOG.md            # Keep-a-Changelog，## [x.y.z] 标题是 release awk 机器契约
├── README.md               # M5
├── docs/plan.md            # M5 从规划文档迁移
├── internal/
│   ├── app/                # Version="0.1.0-dev"、Fail、UserAgent、Bootstrap（M4）
│   ├── api/                # client.go errors.go types.go models.go versions.go tags.go
│   ├── cmd/                # search.go show.go tags.go（M1）vet.go（M2）sync.go login.go prompts.go（M3）
│   ├── config/             # config.go（M3）
│   ├── hashcache/          # cache.go：ComputeSHA256（M2）+ 缓存读写（M3）
│   ├── report/             # table.go json.go format.go
│   ├── env/                # path_windows.go（M4）
│   └── upgrade/            # upgrade.go（M4）
└── testdata/               # 线上 fixture（附录 D 命令再生成）
```

与规划文档 §7 的差异（均为有意决策）：
- 新增 internal/{app,config,env,upgrade}：app/env/upgrade 来自参考基建（用户追加需求）；config 独立成包避免 cmd/login 与 api 互相纠缠
- 省略 api/images.go：v1 无命令消费 /images 浏览端点，--prompts 走版本详情 images[].meta（规划文档 §4.2 功能定义为准）
- hashcache 兼放流式哈希计算（M2 先建计算函数，M3 加缓存），避免为单一函数另立包

## 2. 关键决策

### D1 零依赖 + 手写 dispatch
标准库-only（go.mod 无 require）。main.go `switch os.Args[1]` 路由；每命令 `flag.NewFlagSet`，全局 flag 由共享 helper 注册到每个 FlagSet（--json/--token/--timeout/--verbose）。全局 flag 只接受**命令名之后**的位置（`civ search foo --json`）；`civ --json search` 报用法错误。version/help 支持 -v/--version/-h/--help 别名（此时 -v 无命令上下文，不冲突）。

### D2 API client 三段式（参考项目模式）
- functional options：`NewClient(WithBaseURL, WithHTTPClient, WithToken, WithConcurrency, WithBackoffs...)`；测试同包直改私有字段调短 backoffs，不加测试专用导出 API
- `Client.Get(ctx, path, url.Values)` 唯一咽喉点：UA `civ/<Version> (+https://github.com/BaixuanZhu/civitai-cli)`、Bearer 认证头、并发闸（chan 信号量，容量 = --concurrency，默认 3 上限 8）、重试循环
- 中间结构 `fetchOutcome{body, status, retryAfter, err, fatal}` + 标记错误类型区分可重试/确定性
- 重试语义（规划文档 D2 的"3 次尝试 + 500ms→1s→2s"解读为初始 1 次 + 重试 3 次，退避序列恰 3 项）：backoffs {500ms, 1s, 2s} + ±20% 抖动，封顶 8s；Retry-After（秒数/HTTP-date）覆盖当次等待；重试条件 = 传输错误 / 200 空 body / 429 / 5xx；400/401/403/404 立即返回；所有等待 select ctx.Done()
- URL 一律 url.Values.Encode() 构造，杜绝手拼（静默空响应坑）
- body 上限 io.LimitReader 5MB+1，溢出 fatal

### D3 错误分类与退出码
```
api.ApiError        // 400 ZodError：提取 message 内首个 JSON 片段的 message/values 摘要
api.UnauthorizedError // 401
api.NotFoundError   // 404
api.NetworkError    // 传输/空 body/5xx 重试耗尽
```
退出码映射（cmd 层统一 helper）：0 成功/命中；1 用法错误（flag 解析失败/未知命令/参数缺失）；2 NetworkError；3 NotFoundError（仅 vet 语义；sync 的 not_found 是数据行不是错误，sync 整体 exit 0）；4 UnauthorizedError 且带 token（提示 civ login）；匿名 401（个人态参数）归 1 用法类并提示需 token。

### D4 容错解码
- models 系列 nsfwLevel 用 json.Number 字段（位掩码 1=None 2=Soft 4=Mature 8=X，未知位渲染 "L<bit>"）；images 系列 nsfwLevel 用 string；两套独立不互推
- 枚举一律 string 接收 + 渲染映射函数，映射失败显示原文
- stats 用值结构体 + 缺省零值安全；images[].meta 用 *ImageMeta（null 跳过并计数）
- 未知字段忽略（encoding/json 默认行为）

### D5 baseModel 白名单
M1 实现时先 live 探测 Zod values 泄露拿当前登记枚举（附录 B 机制），钉成常量表 + "verified live 2026-11-22" 注释；--base 未知值 stderr 警告后继续（API 会静默忽略）。sort/type/period 枚举同样钉表并做 CLI 侧映射（most-downloaded → "Most Downloaded" 等，大小写不敏感）。

### D6 全局 -v 冲突解决
规划文档全局 -v（调试）与 show -v <versionId> 冲突。决策：全局调试 flag 为 --verbose（无短别名）；show 的 -v/--ver 保留版本语义（文档示例 UX 优先）。README 注明。

### D7 输出契约
- 人类输出 stdout（text/tabwriter，minwidth 对齐）；日志/进度/警告 stderr（标准库 log，[INFO]/[WARN] 前缀）
- --json：人类装饰全关，search 透传原始 items 数组；show/vet 输出结构化对象；sync 输出行数组；tags 输出名称数组
- 触发词/prompt 配方纯文本块每行一条，无引号无列表符
- k/M 缩写（45.2k）、日期 YYYY-MM-DD、终端宽度 GetConsoleScreenBufferInfo（Windows syscall）取不到按 80、长名截断加 …
- 全英文输出/注释/GoDoc/测试名（参考项目约定，zh-CN 控制台乱码教训）；导出符号必须有 `// Symbol ...` GoDoc

### D8 配置与缓存路径
- config：os.UserConfigDir()/civ/config.json（%AppData%\civ），{"token":"..."}；写入 0600 语义 Windows 尽力而为；优先级 --token > CIVITAI_API_KEY > config
- hashcache：os.UserCacheDir()/civ/cache/hashes.json（%LocalAppData%\civ\cache），map[absPath]{size,mtime,sha256}，原子写（temp+rename）；CIV_NO_CACHE 非空则绕过（集成测试漂移金丝雀需要）；--rehash 全量重算
- login 探针：GET /models?favorites=true&limit=1，200 有效 / 401 无效

### D9 发布基建（M4，参考项目整体移植）
- 版本链：git tag v* → VERSION 剥 v → ldflags -X civitai-cli/internal/app.Version；本地 git describe --tags --always --dirty；兜底 0.1.0-dev
- Bootstrap 链：ldflags -X ...app.Bootstrap=off（dev build）/ CIV_NO_BOOTSTRAP 非空 / exe 在系统 Temp → 跳过 PATH 自举；release build 默认 on
- 产物：civitai-cli-windows-{amd64,arm64}-setup.exe + civitai-cli-windows-{amd64,arm64}.zip（zip 内仅 civ.exe）+ checksums.txt + install.ps1
- upgrade 常量：githubRepo="BaixuanZhu/civitai-cli"、exeName="civ.exe"、资产名 fmt.Sprintf("civitai-cli-%s-%s.zip", runtime.GOOS, runtime.GOARCH)
- NSIS：per-user 免 UAC，InstallDir $LOCALAPPDATA\Programs\civitai-cli，PATH 委托 `civ version` 自举，卸载清 exe/.bak/注册表键/%LOCALAPPDATA%\civ（缓存）/%APPDATA%\civ（config 含 token）
- env.EnsureUserPath：syscall 写 HKCU\Environment + SendMessageTimeoutW 广播 WM_SETTINGCHANGE（不用 setx）；测试 CIV_TEST_REGISTRY=1 门控，仅 CI 隔离 VM 执行
- CI：unit-test 每次 push/PR（windows-latest + go-version-file + vet/test/race/build，env CIV_TEST_REGISTRY=1）；online-smoke 每周 cron + 手动（make build + integration-test.sh，CIV_NO_BOOTSTRAP=1 CIV_NO_CACHE=1）
- release：测试门禁 → 双架构 build-dist → installer-only ×2 → zip ×2 → sha256sum → attestation → awk CHANGELOG notes → gh-release → verify-repro（ubuntu 交叉重建比对 zip 哈希）
- 集成测试脚本：bash 驱动真实 exe 逐子命令断言 exit code + python 解析 --json 形状（search/show/tags/vet 命中/vet 404→3/sync not_found 行）；无 token 依赖

## 3. 数据流（vet 为例）

```
vet 输入分类（64hex→sha256 / 10hex→autov2 / 否则视为路径）
  → 路径则 hashcache.ComputeSHA256 流式（io.Copy 到 sha256.New，32KB buf）
  → api.Get("/model-versions/by-hash/"+hash)（url.PathEscape）
  → 404 → NotFoundError → exit 3（stderr 提示可能非 Civitai 来源或文件被改）
  → 200 → types.ModelVersion 容错解码 → report 渲染审查报告（或 --json）
```

sync：filepath.WalkDir 收集 --ext 文件 → hashcache 批量取哈希（命中跳过计算，文件哈希并发 4）→ API 并发闸 3 逐个 by-hash → 行聚合（同版本多文件标 dupe）→ CSV/JSON/表格输出。

## 4. 兼容性与回滚

- 每个里程碑独立可用（M1 起即完整工具）；子任务按序提交，出问题回滚到上一里程碑 commit
- API 漂移：宽容解码 + fixture 定期重抓（附录 D）+ 每周集成测试金丝雀；漂移修复只改 types/映射表，不动命令层
- 自更新失败回滚：.bak 改名策略自带回滚（新 exe 落位失败 → 恢复 .bak）
- 发布回滚：GitHub Release 可删 tag 重发；verify-repro 失败阻断发布链

## 5. 测试策略

- 单元（每里程碑随代码写）：api 层 httptest fake server（UA/重试计数/退避/错误形态/Retry-After/并发闸）；types 解码表驱动（nsfwLevel 双形态/null meta/空 stats/未知枚举）；cmd 层 mock api 接口 + fixture 断言渲染与退出码；hashcache 临时目录失效判定；upgrade 纯函数 + 假 exe
- fixture：testdata/ 线上抓取（附录 D 命令），注释标抓取日期；CI 绝不触网
- 集成（M4）：真实外网 + 真实 exe，仅 CI 定时/手动 + 本地 make integration
- 注册表测试：CIV_TEST_REGISTRY=1 门控，仅 CI windows runner（隔离 VM）执行，本机禁止