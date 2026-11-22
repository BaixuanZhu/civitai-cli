# danbooru-tag-mcp 基建研究报告（发布/安装/自更新/CI）

> 来源：Explore 代理对 I:\GitDownload\danbooru-tag-mcp 的只读研究，2026-11-22。
> 用途：civitai-cli M4 发布基建的设计依据。路径均相对参考仓库根。

仓库是 Windows-only（GOOS 固定 windows），双架构 amd64/arm64，**不用 goreleaser**，全部手工 make + GitHub Actions。核心亮点是"可复现构建"契约：go.mod 钉死 toolchain（`go 1.26` + `toolchain go1.26.5`）、CGO_ENABLED=0、-trimpath、-buildvcs=false、确定性 zip。

## 1. CI（.github/workflows/ci.yml）

- 触发：push(main)、pull_request、每周一定时 `cron: '0 3 * * 1'`、workflow_dispatch。
- **无 Go 版本矩阵**：单一 `windows-latest`，`setup-go@v7` 用 `go-version-file: go.mod`（精确 toolchain 保证可复现）+ `cache: true`；checkout@v7。
- job1 `unit-test`（每次 push/PR）：`go vet ./...` → `go test ./...` → `go test -race ./...` → `go build ./...`。无 golangci-lint。
- job2 `online-smoke`：`if: github.event_name == 'schedule' || github.event_name == 'workflow_dispatch'`，真实外网测试**永不阻塞发布路径**；env `DANBOORU_MCP_NO_BOOTSTRAP: '1'` 防测试二进制写 runner 的 PATH/注册表；步骤 `make build` + `bash scripts/integration-test.sh`。
- 迁移：civitai CLI 保留"单测每跑、联网烟测定时/手动"分层与隔离 env 思路；env 前缀改 CIV_。

## 2. 发布（.github/workflows/release.yml）

- 触发：`push: tags: ['v*']`。permissions：`contents/id-token/attestations: write`。
- job1 `build-and-release`（windows-latest）：
  1. `go test ./...` 发布前门禁（红则阻断）；
  2. 版本号从 tag 剥离 v：`echo "VERSION=${GITHUB_REF_NAME#v}" >> "$GITHUB_OUTPUT"`；
  3. **同一份构建产物打包两种包装**：`make build-dist VERSION=x GOARCH=amd64` + `arm64`（各建一次到 `dist/<arch>/`），再 `make installer-only`、`make zip` 复用该 exe；
  4. NSIS：`choco install nsis`，choco 不刷新 PATH，手动 `Add-Content $env:GITHUB_PATH 'C:\Program Files (x86)\NSIS'`，随后 `makensis /VERSION` 显式验证；
  5. checksums：`cd dist && sha256sum <4个产物> > checksums.txt`（GNU 格式，裸文件名，upgrade 与 install.ps1 共用）；
  6. `actions/attest-build-provenance@v3` 对 4 个产物做 keyless sigstore 溯源（代替 Authenticode 签名）；
  7. release notes：awk 从 CHANGELOG.md 截取 `## [版本]` 到下一个 `## [` 之间的段落，写入 `$GITHUB_ENV` heredoc；无匹配则 `generate_release_notes: true` 兜底：
```bash
BODY=$(awk -v v="$VERSION" '
  $0 ~ "^## \\[" v "\\]" {found=1; next}
  found && /^## \[/ {exit}
  found {print}' CHANGELOG.md)
```
  8. `softprops/action-gh-release@v3` 上传：2 个 setup.exe、2 个 zip、checksums.txt、**install.ps1 本体**。
- 产物命名：`danbooru-tag-mcp-windows-{amd64|arm64}-setup.exe` / `...-windows-<arch>.zip`（zip 内仅一个 exe）。
- job2 `verify-repro`（ubuntu-latest，needs job1）：Linux 交叉重建同 tag 源码，`gh release download` 拉已发布 zip，逐 arch sha256 对比，结果表写 `$GITHUB_STEP_SUMMARY`，不一致 exit 1。NSIS setup.exe 不可字节复现（makensis 嵌构建元数据），只靠 checksums+attestation 覆盖。

## 3. 自更新（internal/upgrade/upgrade.go + upgrade_test.go）

- 检查 API：`GET https://api.github.com/repos/BaixuanZhu/danbooru-tag-mcp/releases/latest`，头 `User-Agent: app.UserAgent()`、`Accept: application/vnd.github+json`；只解析 tag_name/html_url/assets(name/browser_download_url/size)。
- 版本比较 `compareVersions`：按**非数字字符切段**逐段比整数，缺段计 0，`0.1.0-dev` 后缀段计 0；返回 -1/0/1。本地更新（case 1）跳过，防止 dev/patch 构建被降级。
- 资产名约定：`fmt.Sprintf("danbooru-tag-mcp-%s-%s.zip", runtime.GOOS, runtime.GOARCH)`。
- 下载：zip 存 `os.TempDir()`，重试 3 次线性退避（`time.Sleep(i秒)`）；下载 client `Timeout: 0`（流式 io.Copy 到磁盘，不设总超时以免慢网杀大文件）。
- 校验：下载 release 的 `checksums.txt` 资产，`parseChecksum` 按裸文件名匹配，容忍 sha256sum 二进制模式的 `*` 前缀（Git Bash 默认输出带 `*`）；无 checksums.txt 则警告后继续；hash 不符删临时文件并 Fail。
- **Windows 替换运行中 exe 的核心技巧**（运行中 exe 不可覆盖但**可改名**）：
```go
func replaceExeAt(currentExe, newExePath string) error {
    bakPath := currentExe + ".bak"
    os.Remove(bakPath)                          // 清残留
    if err := os.Rename(currentExe, bakPath); err != nil { return ... } // 1. 旧exe→.bak
    if err := os.Rename(newExePath, currentExe); err != nil {
        os.Rename(bakPath, currentExe)          // 2. 失败回滚
        return ...
    }
    os.Remove(bakPath)                          // 3. 删.bak（进程占用会失败，忽略）
    return nil
}
```
- 两个关键细节：① 新 exe 解压到**当前 exe 所在目录**的临时文件（`os.CreateTemp(selfDir, ...)`），保证 os.Rename 同卷——Windows MoveFileEx 跨卷退化为 copy+delete，对运行中 exe 必失败；② `CleanupStaleBak()` 在**每次启动**静默清理上次升级留下的 `.bak`。
- 接线（main.go）：`case "upgrade": upgrade.Run(); return`；`case "version": fmt.Println("danbooru-tag-mcp", app.Version)`。错误统一 `app.Fail`（stderr + exit 1）。
- 测试全为表驱动纯函数/临时目录假 exe：TestParseChecksum、TestCompareVersions、TestReplaceExeAt（含回滚）、TestExtractSingleExe、TestDownloadOnce（httptest，非 200 视为失败）等，不触网。
- 迁移：改 `githubRepo`、`exeName`、临时文件前缀三个常量即可整体照搬；.bak 策略在 unix 同样无害。

## 4. NSIS（installer/danbooru-tag-mcp.nsi）

- 版本/架构由命令行注入：`makensis /DAPP_VERSION=x.y.z /DAPP_ARCH=arm64`，脚本内 `!ifndef` 兜底 0.0.0/amd64。`OutFile "..\dist\<name>-windows-${APP_ARCH}-setup.exe"`（File 路径相对 nsi 所在目录）。
- **per-user 安装免 UAC**：`InstallDir "$LOCALAPPDATA\Programs\danbooru-tag-mcp"`、`RequestExecutionLevel user`；`InstallDirRegKey HKCU "Software\<name>" "InstallDir"` 复用旧安装目录（与 install.ps1 写同一键）。Unicode true + MUI2 向导（Welcome/Components/Directory/InstFiles/Finish + 卸载确认页）。安装 stub 是 x86，ARM64 上靠系统仿真运行，落地的 exe 是原生架构。
- 三个 Section：
  - SecCore（`SectionIn RO` 必选）：`File "..\dist\${APP_ARCH}\...exe"`、WriteUninstaller、写 InstallDir 键 + HKCU `...\CurrentVersion\Uninstall\<name>` 完整 ARP 条目（DisplayName/Version/Publisher/UninstallString/`QuietUninstallString '"...uninstall.exe" /S'`/NoModify/NoRepair）。
  - SecConfig（PATH，推荐项）：**安装器不自己写 PATH**，委托 exe 自举：`nsExec::ExecToLog '"$INSTDIR\...exe" version'`（nsExec 避免闪控制台窗），触发程序内置 `env.EnsureUserPath`（幂等写 HKCU\Environment + 广播 WM_SETTINGCHANGE）。
  - SecShortcut：开始菜单卸载快捷方式。
- 卸载器：删 exe、**exe.bak**（自更新残留）、uninstall.exe、快捷方式、两个注册表键、`RMDir /r "$LOCALAPPDATA\<name>\cache"`（业务缓存）、RMDir 安装目录。
- 迁移：全局替换名称/注册表键/Publisher/URL；civitai CLI 保留委托模式（main 启动时 EnsureUserPath，dev 构建用 ldflags 关掉）；卸载器 additionally 清理 config（含 token）。

## 5. install.ps1

- 一行安装：`iwr -useb "https://raw.githubusercontent.com/<owner>/<repo>/main/install.ps1" | iex`。**刻意全 ASCII、无 BOM**：iwr|iex 按主机默认代码页解码，UTF-8 BOM 会让 PS 5.1 解析失败（文件头注释明示此约束）。`#requires -Version 5.1`，强制 TLS1.2，`-UseBasicParsing` 免 IE 引擎。
- 参数：`-InstallDir`，默认 `%LOCALAPPDATA%\Programs\<name>`。
- 流程（10 步）：① 64 位检查；② 架构检测用 `[RuntimeInformation]::OSArchitecture`（ARM64 上被仿真的 x64 进程也能报真实 OS 架构，`PROCESSOR_ARCHITECTURE` 仅作老 .NET 兜底）→ amd64/arm64；③ 下载源 `https://github.com/<repo>/releases/latest/download/`，可被 env `DANBOORU_MCP_INSTALLER_MIRROR` 覆盖（镜像）；④ zip+checksums.txt 下到 TEMP 的 GUID 目录；⑤ `Get-ExpectedChecksum` 与 Go parseChecksum 逐行等价（剥 `*`），取不到 checksums 警告继续、取到但无该文件条目则中止，`Get-FileHash` 比对；⑥ 已存在 exe → 先删旧 .bak 再 `Rename-Item` 为 .bak（与 upgrade 同策略，失败提示停进程重试）；⑦ `System.IO.Compression.ZipFile` 按条目名找 exe，`ExtractToFile` 单文件解出；⑧ 写 `HKCU:\Software\<name>\InstallDir`（仅记录，非致命）；⑨ `& $exePath version` 跑一次触发 PATH 自举（失败不回滚，警告）；⑩ 打印后续步骤；finally 清临时目录；catch 友好报错后 rethrow 保证非零退出。
- 迁移：改名/改镜像 env（CIV_INSTALLER_MIRROR）/改结尾提示文案；务必保持 ASCII 无 BOM。

## 6. 集成测试（scripts/integration-test.sh）

- **真实外网**（live Danbooru 匿名），跑的是**build 后的真实二进制**：`EXE="${1:-dist/amd64/....exe}"`，不存在则报错让用户先 make build；断言依赖 python。
- 硬隔离：无条件 `export DANBOORU_MCP_NO_BOOTSTRAP=1`、`NO_CACHE=1`——即使传入 release 味 exe 也不写 PATH/注册表、不吃缓存（缓存回放会掩盖上游 API 漂移，此测试正是漂移金丝雀）。
- 驱动方式（MCP 特有）：18 条 JSON-RPC 请求经管道灌入 exe stdin，stdout 重定向到 mktemp 文件（trap 清理）；末尾 `sleep "$SLEEP"`（默认 60s）撑住 stdin 防 server 提前退出。
- 断言：内嵌 python heredoc 按 id 收集 NDJSON 响应，`check(cond,label)` 累积失败；校验响应形状+具体值，任一失败 `sys.exit` 非零。上游 outage 用 `or [{}]` 兜底让断言干净地 FAIL 而非崩溃。
- CI 调用：仅 online-smoke job（schedule/dispatch），`make build` 后 `bash scripts/integration-test.sh`；本地 `make integration`。
- 迁移：civitai CLI 是一次性命令而非长驻 stdio server，去掉 sleep 撑 stdin 技巧，改为逐条执行 CLI 子命令断言 exit code/stdout JSON；保留"真实外网+定时跑+隔离 env+python 断言"骨架。

## 7. scripts/zipfile（main.go + main_test.go）

- 独立小 Go 程序：构建**字节级可复现 zip**，替代 Compress-Archive（其本地时区 DOS 时间戳破坏复现性）。用法 `zipfile -out <zip> -epoch <unix> <file>...`；文件按参数序放 zip 根、取 base name；每个 header 钉死：
```go
hdr := &zip.FileHeader{Name: filepath.Base(path), Method: zip.Deflate,
    Modified: time.Unix(epoch, 0).UTC()}
hdr.SetMode(0o644)   // 固定模式位，无 extra fields
```
- epoch 来自 Makefile：`SOURCE_DATE_EPOCH ?= $(shell git log -1 --pretty=%ct || echo 946684800)`（提交时间，reproducible-builds.org 惯例，可 export 覆盖）。
- 调用陷阱：`env -u GOOS -u GOARCH go run ./scripts/zipfile ...`——make 命令行变量 `GOARCH=arm64` 会导出到 recipe，连打包器自身都被交叉编译而在 runner 上跑不起来，故 env -u 剥掉。
- 迁移：完全通用，可原样拷贝。

## 8. Makefile

变量：`BINARY`；`VERSION := $(shell git describe --tags --always --dirty | sed 's/^v//')`，空则 `0.1.0-dev`；`GOARCH ?= $(shell go env GOARCH)`；产物按架构分目录 `dist/<arch>/<bin>.exe` 防双架构互踩。核心注入：

```make
LDFLAGS     := -s -w -X danbooru-tag-mcp/internal/app.Version=$(VERSION)
LDFLAGS_DEV := $(LDFLAGS) -X danbooru-tag-mcp/internal/app.Bootstrap=off
# build 配方：
GOOS=windows GOARCH=$(GOARCH) CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags "$(LDFLAGS_DEV)" -o $(TARGET) .
```

全部 target：`all`→build；`build`（dev 味，Bootstrap=off 不污染用户 PATH）；`build-dist`（release 味，无 off）；`run ARGS="..."`；`release`（build-dist+zip+`sha256sum $$(ls *.zip) > checksums.txt`，双架构重跑合并同一清单）；`zip`（确定性打包已建 exe，不重建）；`installer`（build-dist+installer-only）；`installer-only`（`MSYS_NO_PATHCONV=1 makensis /DAPP_VERSION=$(VERSION) /DAPP_ARCH=$(GOARCH) installer/....nsi`，MSYS 变量防 Git Bash 把 /D 当 POSIX 路径改写）；`dist-all`（installer+release 一键）；`clean`；`tidy`；`fmt`；`vet`；`test`；`test-race`；`integration`（build+脚本）；`help`。

## 9. 版本号管理与 CHANGELOG

- 唯一版本源：`internal/app/app.go` 包级 `var Version = "0.1.0-dev"`（未注入兜底），构建时 -X 覆盖；同包 `Fail()`（stderr+exit 1）与 `UserAgent()`（`<name>/<Version>`，upgrade 复用）。放 internal/app 避免 env/upgrade import 成环。`bootstrap.go` 的 `var Bootstrap = "on"` 同一 ldflags 约定，`BootstrapEnabled()` 综合判断：ldflags off / env NO_BOOTSTRAP 非空 / exe 位于系统 Temp（go run 临时二进制）则跳过。
- main.go version 子命令引用 app.Version，全链路一致；源码不硬编码版本字面量（除兜底）。发布版本 = git tag `v0.2.0` 剥 v；本地 = git describe（含 -dirty）。
- CHANGELOG.md：Keep-a-Changelog 风格，`## [x.y.z] - YYYY-MM-DD`（开发中写 `- Unreleased`，打 tag 时替换日期），小节 `### Added / Changed / Breaking`；release.yml awk 按 `## [版本]` 精确截取——**标题格式是机器契约，不能随意改**。

## 总体迁移清单（civitai-cli 视角）

1. 直接照搬：upgrade 包（改 3 常量）、zipfile、Makefile 骨架、CHANGELOG 契约、release.yml 的 tag→VERSION→ldflags→checksums→awk notes 链。
2. 已决策：Windows-only amd64+arm64（与参考一致），CI 全 windows-latest。
3. 需重写：integration-test.sh 驱动层（MCP stdio 管道 → CLI 子命令调用），断言层可留。
4. PATH 自举保留委托模式（internal/env：syscall 写 HKCU\Environment + SendMessageTimeoutW 广播 WM_SETTINGCHANGE）。
5. 保留 attestation + verify-repro（确定性构建/打包 + go.mod toolchain 钉死）。