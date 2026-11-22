# M1 执行清单

> 顺序执行；每步完成即跑对应验证。外网动作仅限步骤 2（fixture 抓取 + 枚举探测），其余全本地。

## 准备

- [ ] 0. git init（本地仓库，无 remote；.gitattributes 先行防 CRLF 污染）+ 首个 commit 前征求用户同意
- [ ] 1. 骨架：go.mod（module civitai-cli / go 1.27 / toolchain go1.27.3）、.gitattributes、LICENSE（MIT，版权行 BaixuanZhu）、internal/app/app.go（Version/Fail/UserAgent）、main.go dispatch 雏形（version/help/unknown）、Makefile 基础 target
  - 验证：`make build && ./dist/amd64/civ.exe version`

## API 层

- [ ] 2. live 抓取 fixture（附录 D 全部 7 条 curl）+ 枚举探测（baseModel/__invalid__ 取 Zod values；sort/type 合法值核对）→ testdata/ + 注释日期
  - 验证：7 个 json 文件存在且 python json.loads 通过
- [ ] 3. internal/api/errors.go：双形态解析 + 四错误类型 + Zod 摘要
  - 验证：errors_test.go 表驱动（附录 B 三样例 + 非 JSON body）
- [ ] 4. internal/api/types.go + enums.go：容错结构体、nsfwLevel 双形态、枚举映射表、baseModel 白名单（步骤 2 钉表）
  - 验证：types_test.go（fixture 解码 + 畸形 JSON 表驱动）
- [ ] 5. internal/api/client.go：options/Get 咽喉点/重试/并发闸/Retry-After/空 body/5MB 上限
  - 验证：client_test.go（httptest countingServer：UA/Bearer/重试次数/4xx 不重试/Retry-After/并发闸）
- [ ] 6. internal/api/models.go versions.go tags.go：端点方法（url.Values 构造，返回 raw body）+ api.ParseXxx 解码 helper
  - 验证：端点测试断言请求 URL 编码正确（sort 空格 %20、types 多值、limit）

## 命令层

- [ ] 7. internal/report：table.go（tabwriter/截断/终端宽度）、format.go（k/M/日期/ratio/NSFW 位掩码渲染）、json.go
  - 验证：format_test.go 表驱动 + 宽度回退 80
- [ ] 8. internal/cmd/search.go + 全局 flag helper + 退出码映射
  - 验证：cmd 测试（httptest fixture：表格关键行/--json 透传/未知 base 警告/limit>100 exit 1/未知 sort exit 1）
- [ ] 9. internal/cmd/show.go（模型头+版本表+ -v 钻版本+触发词块）
  - 验证：cmd 测试（fixture 渲染/版本钻取/触发词每行一条）
- [ ] 10. internal/cmd/tags.go
  - 验证：cmd 测试（名称列表/--json 数组）

## 收尾

- [ ] 11. 全量质量门：`go vet ./... && go test ./... && go test -race ./...`；`gofmt -l .` 为空
- [ ] 12. live 冒烟（手动，开发期允许）：search stocking --type lora --sort most-downloaded --period month；search anima --json | python 解析；show 2458426 / show -v；tags anima
- [ ] 13. README 占位（M5 补全）可跳过；usage() 帮助文本含全部已实现命令与全局 flag
- [ ] 14. trellis-check 子代理质量检查 → prd.md 验收清单核对 → 提醒 commit（用户确认）

## 回滚点

- 步骤 1 骨架 commit 后，API 层/命令层各自 commit；任一步骤失败回滚到上一 commit 重做

## 验证命令

```bash
go vet ./... && go test ./... && go test -race ./...
make build
./dist/amd64/civ.exe search stocking --type lora --limit 5
./dist/amd64/civ.exe search anima --json > /tmp/s.json && python -c "import json;json.load(open('/tmp/s.json'))"
./dist/amd64/civ.exe show 2458426
./dist/amd64/civ.exe tags anima
```