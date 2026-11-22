# M1 骨架与核心命令：API client + search/show/tags

## Goal

从零搭建 civitai-cli 项目骨架与 API 基础设施，交付 search/show/tags 三个只读命令。M1 完成即每日可用工具（规划文档 §8）。

## Requirements

### 骨架

- go.mod：module `civitai-cli`，`go 1.27` + `toolchain go1.27.3`，零第三方依赖
- .gitattributes：`* text=auto eol=lf`，exe/zip binary，journal merge=union（照参考项目）
- main.go：仅 dispatch（switch os.Args[1]）+ 全局 flag 装配 helper；version/help 命令（version 引用 internal/app.Version）；未知命令 stderr + usage + exit 1；log 输出 stderr
- internal/app：Version="0.1.0-dev"（ldflags 注入点）、Fail（stderr+exit 1）、UserAgent()
- Makefile 基础 target：build（dev 味 ldflags）/test/test-race/vet/fmt/tidy/run/clean/help；VERSION 来自 git describe（无 git 时兜底 0.1.0-dev）；CGO_ENABLED=0 -trimpath -buildvcs=false；产物 dist/<arch>/civ.exe
- internal/report：tabwriter 封装、k/M 缩写、YYYY-MM-DD、终端宽度（GetConsoleScreenBufferInfo，失败按 80）、长名截断

### API client（internal/api，父 design D2-D5）

- client.go：functional options、Get 唯一咽喉点、UA/Bearer、并发闸（默认 3 上限 8）、重试（初始+3 重试，500ms/1s/2s+抖动 cap 8s，Retry-After，空 body/429/5xx/传输错误重试，4xx 不重试）、url.Values 编码、5MB body 上限
- errors.go：ApiError（Zod 摘要）/UnauthorizedError/NotFoundError/NetworkError，双错误形态解析（附录 B）
- types.go：Model/ModelVersion/ModelFile/Image/ImageMeta/Stats/Creator 等容错结构体；nsfwLevel 数字（json.Number）与字符串双形态；meta 可 null；未知枚举原样保留
- models.go：GET /models（query/types/baseModel/tag/username/sort/period/limit/cursor）、GET /models/{id}
- versions.go：GET /model-versions/{id}、GET /model-versions/by-hash/{hash}（M2 消费，M1 先建）
- tags.go：GET /tags?query=&limit=
- 枚举映射表：sort/period/type 的 CLI 值 → API 值（大小写不敏感）；baseModel 白名单（live 探测 Zod values 钉表，警告不阻断）

### 命令（internal/cmd）

- search.go：--type（可重复/逗号分隔）/--base/--sort（默认 hot）/--period（默认 all）/--tag/--user/--limit（≤100 默认 20）；表格列 ID/NAME/TYPE/BASE/DL/RATIO/UPDATED/NSFW/CREATOR；--json 透传原始 items
- show.go：模型头（名称/创作者/类型/标签/许可四件套/统计/更新时间/URL）+ 版本表；-v/--ver <versionId> 钻版本（文件表含全量哈希/扫描结果/触发词纯文本块）；--prompts 留 M3
- tags.go：前 N 条名称列表（--limit 默认 20）；--json 名称数组
- 全局 flag：--json/--token/--timeout（默认 30s）/--verbose；token 此阶段仅 --token flag 与 CIVITAI_API_KEY 环境变量（config 文件 M3）
- 退出码：0/1/2/4（3 留 M2 vet）

### fixture 与测试

- testdata/：附录 D 命令 live 抓取（models_search_anima/models_2458426/version_3263843/byhash_anima_turbo/err_limit_too_big/err_not_found/err_unauthorized），文件头注释抓取日期
- 单元测试零外网：client（httptest：UA/重试计数/空 body 重试/429 Retry-After/5xx 重试/4xx 不重试/并发闸）、errors（Zod 摘要/字符串形态）、types（nsfwLevel 双形态/null meta/空 stats/未知枚举）、cmd（flag 映射/URL 编码断言/baseModel 警告/表格渲染/--json 透传）

## Acceptance Criteria

- [ ] `go vet ./...`、`go test ./...`、`go test -race ./...` 全绿，测试零外网
- [ ] `make build` 产出 dist/amd64/civ.exe；`civ version` 打印版本
- [ ] `civ search stocking --type lora --sort most-downloaded --period month --limit 20` 出表（live 冒烟，80 列内不折行失控）
- [ ] `civ search anima --json` 可被 python json.loads 解析且为 items 数组
- [ ] `civ show 2458426` 出模型头+版本表；`civ show 2458426 -v <id>` 出文件表+触发词
- [ ] `civ tags anima` 出名称列表
- [ ] 未知 --base 值 stderr 警告；limit>100 CLI 侧拦截（exit 1）
- [ ] go.mod 零 require；导出符号全 GoDoc；全英文输出

## Notes

- 事实源：research/civitai-cli-plan.md §3/§4.1/§4.2/§4.5/附录 A-D；父 design.md D1-D7
- 参考代码组织：research/danbooru-code-org.md（client 三段式/测试模式/注释约定）
- live 抓取 fixture 与枚举探测是本任务唯一允许的外网动作（开发期手动，CI 永不触网）