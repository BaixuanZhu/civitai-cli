# 执行计划 — civitai-cli 父任务

> 父任务不做直接实现，拥有任务地图、跨子任务验收与最终集成审查。实现按子任务顺序推进。

## 子任务执行顺序

```
11-22-civ-m1-core → 11-22-civ-m2-vet → 11-22-civ-m3-sync-login → 11-22-civ-m4-release → 11-22-civ-m5-docs
```

依赖是顺序的（每个里程碑构建在前一个之上），各子任务 prd.md/implement.md 内含自己的验收与验证命令。

## 每个子任务的循环

1. `task.py start <child>`（工件审查通过后）
2. 实现（trellis-implement 子代理或主代理 inline；dispatch prompt 首行带 Active task 路径）
3. 质量检查（trellis-check：go vet / go test / go test -race / 手工冒烟对应命令）
4. 子任务验收核对 → 归档提醒（commit 由用户确认后执行；本仓库当前无 git remote，首次 commit 前需 git init 确认）

## 里程碑门禁

| 门禁 | 验证 | 阻断条件 |
|---|---|---|
| M1 完成 | go test ./... 零外网全绿；civ search stocking --type lora 出表；--json 可被 python 解析 | 测试触网 / 表格失控折行 |
| M2 完成 | 已知哈希（附录 D Anima SHA256）vet 出报告 exit 0；DEADBEEF... vet exit 3 | 退出码语义不符 |
| M3 完成 | 临时目录合成文件 sync 出 CSV；二次运行哈希缓存命中；login 探针 httptest 200/401 | 缓存失效判定错误 |
| M4 完成 | make dist-all 产 4 产物 + checksums；zip 双跑字节一致；upgrade 单测全绿；注册表测试本机跳过 | 产物缺失 / zip 不可复现 |
| M5 完成 | README 全命令示例可复制执行；规划文档 §10 逐条核对表 | 验收缺项 |

## 最终集成审查（M5 后，父任务归档前）

- [ ] prd.md 跨子任务验收清单逐条核对（含性能项：真实目录可用则实测，否则合成基准 + 注明）
- [ ] go.mod 零 require；go build CGO_ENABLED=0 单文件
- [ ] 全命令 --json 契约抽查（python json.loads）
- [ ] 退出码矩阵抽查（0/1/2/3/4 各一例）
- [ ] CI/release 工作流 actionlint 或语法审查；真实 tag 发布留给用户（外部可见动作，先征求同意）
- [ ] spec 更新（.trellis/spec/backend 填入本项目实际约定：分层/错误处理/测试/可复现构建）
- [ ] LICENSE（MIT，已确认）在 M1 骨架放入，M5 核对

## 回滚点

- 每里程碑一个 commit（用户确认后）；回滚 = revert 到上一里程碑
- M4 发布链失败不污染已发布产物（verify-repro 在 release job 之后独立跑，失败阻断的是"验证通过"标记而非产物上传——与参考项目一致）

## 验证命令汇总

```bash
go vet ./... && go test ./... && go test -race ./...
make build && ./dist/amd64/civ.exe search stocking --type lora --limit 5
make dist-all   # M4 后
bash scripts/integration-test.sh   # 真实外网，本地手动/CI 定时
```