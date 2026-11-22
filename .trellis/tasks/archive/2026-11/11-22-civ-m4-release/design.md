# M4 技术设计 — 发布基建

> 移植蓝本：research/danbooru-infra.md（含关键代码片段）。本文只列 civitai-cli 侧的差异化决策与接线。

## 1. 命名与常量表

| 项 | 值 |
|---|---|
| module | civitai-cli |
| 二进制 | civ.exe |
| githubRepo | BaixuanZhu/civitai-cli |
| 产物 | civitai-cli-windows-{amd64,arm64}-setup.exe / civitai-cli-windows-{amd64,arm64}.zip |
| zip 内 | 仅 civ.exe |
| upgrade 资产名 | fmt.Sprintf("civitai-cli-%s-%s.zip", runtime.GOOS, runtime.GOARCH) |
| 安装目录 | $LOCALAPPDATA\Programs\civitai-cli |
| 注册表键 | HKCU\Software\civitai-cli（InstallDir）+ ARP Uninstall\civitai-cli |
| 缓存目录 | %LOCALAPPDATA%\civ\cache（os.UserCacheDir()/civ） |
| 配置目录 | %APPDATA%\civ（os.UserConfigDir()/civ） |
| 隔离 env | CIV_NO_BOOTSTRAP / CIV_NO_CACHE / CIV_TEST_REGISTRY / CIV_INSTALLER_MIRROR |
| ldflags | -X civitai-cli/internal/app.Version / -X civitai-cli/internal/app.Bootstrap=off（dev） |

## 2. internal/app 扩展

```go
var Version = "0.1.0-dev"   // M1 已有
var Bootstrap = "on"        // M4 新增，ldflags 注入点
func BootstrapEnabled() bool // Bootstrap != "off" && os.Getenv("CIV_NO_BOOTSTRAP") == "" && !underSystemTemp(exePath)
```

- underSystemTemp：os.Executable() 解析后与 os.TempDir() 前缀比较（go run 临时二进制不污染 PATH）
- app 包保持无 internal 依赖（env/upgrade 都 import app，避免环）

## 3. internal/env（path_windows.go）

- 移植参考 env.EnsureUserPath：registry.OpenKey HKCU\Environment（SET_VALUE|QUERY_VALUE）→ 读 PATH（不存在则建）→ 分号 split 检查 exe 目录是否已在（大小写不敏感比较）→ 追加 → registry.SetStringValue → SendMessageTimeoutW(HWND_BROADCAST, WM_SETTINGCHANGE, 0, "Environment", SMTO_ABORTIFHUNG, 5000)
- syscall 常量自声明（golang.org/x/sys 不引——零依赖约束；参考项目同样手写 syscall）
- 失败返回 error，main 侧 [WARN] 降级不致命
- 测试：纯函数部分（PATH 合并/去重/大小写）本机跑；真实注册表读写测试 CIV_TEST_REGISTRY=1 门控（t.Skip 默认），仅 CI windows runner 执行

## 4. internal/upgrade

- 整体移植参考 upgrade.go，改常量表三项；逻辑不动：compareVersions 非数字切段、本地更新跳过、checksums 容忍 * 前缀、同卷 CreateTemp(selfDir)、.bak 三步替换带回滚、CleanupStaleBak 启动清理
- Run() 流程：查 latest → 比较 → 无更新打印当前版本 exit 0 → 下载 zip+checksums → 校验 → 解压 civ.exe 到 selfDir 临时文件 → replaceExeAt → 打印新版本提示重启
- main.go 接线：启动无条件 CleanupStaleBak()（静默）；case "upgrade" → upgrade.Run()

## 5. Makefile 完整版（M1 基础 + 新增）

```make
LDFLAGS_DEV := $(LDFLAGS) -X civitai-cli/internal/app.Bootstrap=off
SOURCE_DATE_EPOCH ?= $(shell git log -1 --pretty=%ct 2>/dev/null || echo 946684800)
build-dist:  # 同 build 但用 LDFLAGS（无 off）
zip:         # env -u GOOS -u GOARCH go run ./scripts/zipfile -out dist/civitai-cli-windows-$(GOARCH).zip -epoch $(SOURCE_DATE_EPOCH) dist/$(GOARCH)/civ.exe
installer-only:  # MSYS_NO_PATHCONV=1 makensis /DAPP_VERSION=$(VERSION) /DAPP_ARCH=$(GOARCH) installer/civitai-cli.nsi
release: build-dist zip  # + sha256sum 合并清单（双架构重跑合并同一 checksums.txt）
dist-all: installer release
integration: build  # bash scripts/integration-test.sh
```

- 陷阱备忘：make 命令行 GOARCH=arm64 导出到 recipe → zipfile 自身被交叉编译跑不起来 → env -u 剥掉；Git Bash 把 makensis /D 当路径 → MSYS_NO_PATHCONV=1

## 6. NSIS 差异化

- 参考脚本全局替换：名称 civitai-cli/civ.exe、Publisher BaixuanZhu、URL github.com/BaixuanZhu/civitai-cli
- 卸载器比参考多清 config：`RMDir /r "$APPDATA\civ"`（含 token，卸载即清除敏感数据）+ `$LOCALAPPDATA\civ`（缓存）
- SecConfig 委托 `civ.exe version`（M1 已有 version 命令，启动接线 M4 补 Bootstrap）

## 7. install.ps1 差异化

- 参考脚本改：名称/URL/镜像 env CIV_INSTALLER_MIRROR/结尾提示（civ search 示例）
- 保持 ASCII 无 BOM； checksum 逻辑与 Go parseChecksum 逐行等价（剥 *）
- 验证：python 读字节断言全 <128 且无 BOM；powershell -NoProfile -Command "[scriptblock]::Create((Get-Content -Raw install.ps1))" 语法解析（不执行）

## 8. CI/release 工作流

- 照参考 ci.yml/release.yml 结构，替换：名称/env 前缀/产物名/go-version-file 读本仓库 go.mod
- ci unit-test env 增 CIV_TEST_REGISTRY=1（windows runner 隔离 VM，符合用户注册表测试规则）
- online-smoke：CIV_NO_BOOTSTRAP=1 CIV_NO_CACHE=1 + make build + bash scripts/integration-test.sh
- release verify-repro：ubuntu-latest 交叉 GOOS=windows GOARCH=amd64/arm64 重建 → 与已发布 zip sha256 比对（zip 确定性由 zipfile+SOURCE_DATE_EPOCH 保证；SOURCE_DATE_EPOCH 取 tag commit 时间，两边一致）
- 注意：本仓库无 remote 前工作流无法真跑；M4 验收以本地 make + YAML 语法 + 逻辑审查为准，真实 CI 留用户建仓推送后

## 9. integration-test.sh 驱动层重写

- 参考是 MCP stdio 管道灌 JSON-RPC；civ 是一次性命令 → 逐条执行断言：
```bash
run() { "$EXE" "$@" >"$OUT" 2>"$ERR"; echo $?; }  # 收集 exit code
# search --json → python 断言 items 数组非空/含 id,name,type
# vet <已知SHA256> → exit 0；vet DEADBEEF00 → exit 3
# sync 临时目录（脚本内 mktemp 造 2 个 dummy 文件）→ exit 0 + CSV 含 not_found
```
- python heredoc 断言层保留参考风格（check 累积失败、sys.exit 非零、outage 兜底干净 FAIL）
- 无条件 export CIV_NO_BOOTSTRAP=1 CIV_NO_CACHE=1；trap 清理临时文件