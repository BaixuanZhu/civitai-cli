# M4 发布基建：CI/自动发布/自更新/安装包

## Goal

参考 danbooru-tag-mcp（research/danbooru-infra.md）补齐 civitai-cli 的发布与分发基建：可复现构建、GitHub Actions CI/自动发布、自更新、NSIS 安装包、install.ps1、真实外网集成测试。

## Requirements

### 可复现构建与版本链

- Makefile 完整 target：build（dev 味 Bootstrap=off）/build-dist（release 味）/zip/installer/installer-only/release/dist-all/integration/test-race/clean/tidy/fmt/vet/run/help
- ldflags：-s -w -X civitai-cli/internal/app.Version=$(VERSION)；dev 追加 -X ...app.Bootstrap=off
- VERSION := git describe --tags --always --dirty 剥 v，兜底 0.1.0-dev；SOURCE_DATE_EPOCH := git log -1 --pretty=%ct 兜底 946684800
- scripts/zipfile：确定性 zip 原样移植（header 钉死 Modified/Mode，env -u GOOS -u GOARCH go run 调用陷阱）
- CHANGELOG.md：Keep-a-Changelog，## [0.1.0] - Unreleased 起步；标题格式是 release awk 机器契约

### 自更新（internal/upgrade）

- 移植参考实现，改三常量：githubRepo="BaixuanZhu/civitai-cli"、exeName="civ.exe"、临时文件前缀
- releases/latest 检查 → compareVersions（本地更新跳过）→ 资产 civitai-cli-windows-<arch>.zip 下载（3 次线性退避，Timeout 0 流式）→ checksums.txt SHA256 校验（容忍 * 前缀，缺失警告继续）→ 同卷临时文件解压 → .bak 改名替换（失败回滚）
- main.go：case "upgrade"；每次启动 upgrade.CleanupStaleBak()
- 单测全移植：ParseChecksum/CompareVersions/ReplaceExeAt（含回滚）/ExtractSingleExe/DownloadOnce（httptest）

### PATH 自举（internal/env + internal/app.Bootstrap）

- app.Bootstrap="on" 变量 + BootstrapEnabled()：ldflags off / CIV_NO_BOOTSTRAP 非空 / exe 在系统 Temp → 跳过
- env.EnsureUserPath：syscall 写 HKCU\Environment PATH（幂等追加 exe 目录）+ SendMessageTimeoutW 广播 WM_SETTINGCHANGE；不用 setx
- main.go 启动接线：BootstrapEnabled() → EnsureUserPath()（失败 [WARN] 不致命）
- **注册表测试 CIV_TEST_REGISTRY=1 门控，仅 CI 隔离 VM 执行，本机禁止**（用户既定规则）；CI unit-test job env 设 CIV_TEST_REGISTRY=1

### NSIS 安装包（installer/civitai-cli.nsi）

- per-user 免 UAC：RequestExecutionLevel user、InstallDir $LOCALAPPDATA\Programs\civitai-cli、InstallDirRegKey HKCU Software\civitai-cli
- /DAPP_VERSION /DAPP_ARCH 注入，!ifndef 兜底；OutFile ..\dist\civitai-cli-windows-${APP_ARCH}-setup.exe
- SecCore：File civ.exe + WriteUninstaller + ARP 完整条目（含 QuietUninstallString /S）
- SecConfig：nsExec::ExecToLog '"$INSTDIR\civ.exe" version' 触发 PATH 自举
- SecShortcut：开始菜单卸载快捷方式
- 卸载器：exe/exe.bak/uninstall.exe/快捷方式/注册表键 + RMDir /r $LOCALAPPDATA\civ（缓存）+ $APPDATA\civ（config 含 token）

### install.ps1

- 全 ASCII 无 BOM（文件头注释约束）；#requires 5.1；TLS1.2；-UseBasicParsing
- 架构检测 [RuntimeInformation]::OSArchitecture；下载源 github.com/BaixuanZhu/civitai-cli/releases/latest/download/，CIV_INSTALLER_MIRROR 可覆盖
- checksum 校验（与 Go parseChecksum 等价）→ .bak 策略 → ZipFile 单文件解出 → 写 InstallDir 键 → & civ.exe version 触发自举 → 友好报错 rethrow

### CI（.github/workflows/ci.yml）

- 触发：push(main)/PR/每周 cron '0 3 * * 1'/workflow_dispatch
- unit-test（windows-latest，setup-go go-version-file+cache，checkout）：vet → test → test -race → build；env CIV_TEST_REGISTRY=1（runner 是隔离 VM）
- online-smoke（仅 schedule/dispatch）：CIV_NO_BOOTSTRAP=1 CIV_NO_CACHE=1；make build + bash scripts/integration-test.sh；永不阻塞发布路径

### 发布（.github/workflows/release.yml）

- 触发 push tags v*；permissions contents/id-token/attestations write
- build-and-release（windows-latest）：go test 门禁 → VERSION 剥 v → build-dist amd64+arm64 → choco nsis + GITHUB_PATH 手动追加 + makensis /VERSION 验证 → installer-only ×2 → zip ×2 → sha256sum 4 产物 > checksums.txt → attest-build-provenance@v3 → awk CHANGELOG notes（无匹配 generate_release_notes 兜底）→ softprops/action-gh-release@v3 上传 2 setup.exe + 2 zip + checksums.txt + install.ps1
- verify-repro（ubuntu-latest，needs build-and-release）：交叉重建同 tag → gh release download zip → 逐 arch sha256 比对 → STEP_SUMMARY 表 → 不一致 exit 1

### 集成测试（scripts/integration-test.sh）

- bash 驱动真实 exe（参数传入或默认 dist/amd64/civ.exe），python 断言；无条件 export CIV_NO_BOOTSTRAP=1 CIV_NO_CACHE=1
- 用例：search --json 形状（items 数组/关键字段）、show --json、tags --json、vet 已知 SHA256 exit 0、vet DEADBEEF00 exit 3、sync 临时目录 not_found 行 exit 0、version 输出
- 无 token 依赖（匿名读取面）；上游 outage 干净 FAIL 不崩溃

## Acceptance Criteria

- [ ] make dist-all 产出 dist/civitai-cli-windows-{amd64,arm64}-setup.exe + .zip ×2 + checksums.txt（本机需 NSIS；无 NSIS 则 zip+checksums 验证，setup 留 CI）
- [ ] zip 确定性：同 SOURCE_DATE_EPOCH 双跑字节一致（sha256 比对）
- [ ] upgrade 单测全绿（版本比较/checksum/.bak 回滚/解压/下载 httptest）；本机 go test 不触注册表（CIV_TEST_REGISTRY 未设跳过）
- [ ] dev build（make build）运行不写 PATH；CIV_NO_BOOTSTRAP=1 release 味 exe 也不写
- [ ] install.ps1 全 ASCII 无 BOM（python 断言字节）；PS 5.1 语法解析通过
- [ ] ci.yml/release.yml YAML 语法正确；unit-test 路径零外网；online-smoke 仅定时/手动
- [ ] CHANGELOG ## [0.1.0] 节存在且 awk 可截取
- [ ] integration-test.sh 本地 live 跑通（开发期手动）
- [ ] 真实 tag 发布与 verify-repro 留待用户推 tag（外部可见动作，先征求同意）

## Notes

- 设计细节见 design.md；移植源 I:\GitDownload\danbooru-tag-mcp（只读参考，不修改）
- NSIS 本机可能未安装：installer target 失败不阻断其他验收，CI 会补验