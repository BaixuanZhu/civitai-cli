# M4 执行清单

> 移植源 I:\GitDownload\danbooru-tag-mcp 只读。顺序：版本链 → 自更新/自举 → 打包 → 安装器 → CI → 集成脚本。

- [ ] 1. internal/app 扩展：Bootstrap 变量 + BootstrapEnabled()（ldflags off/env/Temp 三条件）+ 测试（纯函数部分）
- [ ] 2. internal/env/path_windows.go：EnsureUserPath（syscall 注册表 + 广播）；PATH 合并纯函数测试本机跑；注册表测试 CIV_TEST_REGISTRY=1 门控 t.Skip
  - 验证：go test ./internal/env/ 本机全绿（注册表用例跳过）；**禁止本机设 CIV_TEST_REGISTRY=1**
- [ ] 3. internal/upgrade：移植 + 三常量替换 + 测试全移植
  - 验证：go test ./internal/upgrade/（httptest 下载/版本比较表驱动/.bak 回滚/单文件解压）
- [ ] 4. main.go 接线：启动 CleanupStaleBak + BootstrapEnabled→EnsureUserPath（[WARN] 降级）；case "upgrade"；usage 更新
  - 验证：make build（dev）运行 version 不写 PATH（注册表查询确认前后一致——只读查询本机允许）
- [ ] 5. scripts/zipfile 移植 + Makefile 完整 target（build-dist/zip/installer/installer-only/release/dist-all/integration）+ CHANGELOG.md（## [0.1.0] - Unreleased）
  - 验证：make zip 双跑 sha256 一致；awk 截取 CHANGELOG 节成功
- [ ] 6. installer/civitai-cli.nsi：移植替换 + 卸载清 config/cache
  - 验证：本机有 NSIS 则 make installer-only 产 setup.exe；无则跳过留 CI（prd 已注明）
- [ ] 7. install.ps1：移植替换（ASCII 无 BOM）
  - 验证：python 字节断言全 ASCII 无 BOM；PS 语法解析通过（不执行）
- [ ] 8. .github/workflows/ci.yml + release.yml：移植替换（env 前缀/产物名/CIV_TEST_REGISTRY=1 仅 CI）
  - 验证：python yaml.safe_load 两文件；逻辑对照参考逐段审查
- [ ] 9. scripts/integration-test.sh：驱动层重写为 CLI 子命令断言（search/show/tags/vet 命中/vet 404/sync/version）
  - 验证：本地 live 跑通（make integration，开发期手动外网）
- [ ] 10. 全量质量门：go vet / go test / go test -race（注册表用例本机跳过）；make dist-all（或无 NSIS 降级路径）
- [ ] 11. trellis-check → 验收核对 → commit 提醒（用户确认）；真实 tag 发布/CI 真跑留用户建仓后（外部可见动作先征求同意）

## 验证命令

```bash
go test ./... && go test -race ./...
make build && ./dist/amd64/civ.exe version
make zip && make zip && sha256sum dist/*.zip   # 双跑一致
make dist-all   # 需 NSIS；无则 make release
python -c "d=open('install.ps1','rb').read(); assert all(b<128 for b in d) and not d.startswith(b'\xef\xbb\xbf')"
python -c "import yaml;yaml.safe_load(open('.github/workflows/ci.yml'));yaml.safe_load(open('.github/workflows/release.yml'))"
make integration   # live 外网
```

## 回滚点

- M3 commit 基线；自更新/自举一个 commit，打包/安装器一个 commit，CI/集成脚本一个 commit
- main.go 接线若致启动异常，回滚接线 commit（upgrade/env 包独立可留）