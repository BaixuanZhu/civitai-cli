# M5 执行清单

- [ ] 1. README.md 按 prd 结构撰写（英文；示例与真实 flag/产物名核对——读 main.go usage 与 Makefile 确认）
- [ ] 2. docs/plan.md 迁移 + 头部实现状态注记（v1 范围、D6 -v 冲突决策、images.go 省略、hashcache 兼放计算）
- [ ] 3. LICENSE 核对：MIT 已在 M1 放入，确认版权行与年份
- [ ] 4. 全命令 --json 契约抽查：search/show/tags/vet/sync 各跑一次 python json.loads
- [ ] 5. 退出码矩阵抽查：0（search 正常）/1（非法 flag）/2（断网或不可达 base URL 模拟——用 --timeout 极短 + 无效 host？不引入测试专用 flag：httptest 单测已覆盖，live 用 vet 404→3、非法输入→1、正常→0；2/4 以单测为据注明）/3（vet DEADBEEF00）/4（login 无效 key httptest 或 live 无效 token）
- [ ] 6. 性能核对表：vet 计时（live 已知哈希）；sync 50 文件首跑/二次计时（真实目录或合成基准，注明条件）
- [ ] 7. 规划文档 §10 逐条核对表写入本任务 research/acceptance.md
- [ ] 8. .trellis/spec/backend：directory-structure.md / error-handling.md / quality-guidelines.md / logging-guidelines.md 填实（英文，从父 design.md 提炼实际约定）
- [ ] 9. 父任务集成审查（implement.md 清单）→ 全勾后父任务与子任务归档提醒
- [ ] 10. 最终 commit 提醒（用户确认）；建仓推送/首次 tag 发布属外部可见动作，单独征求同意

## 验证命令

```bash
go vet ./... && go test ./... && go test -race ./...
make build
./dist/amd64/civ.exe search anima --json | python -c "import json,sys;json.load(sys.stdin)"
./dist/amd64/civ.exe vet DEADBEEF00; echo exit=$?   # 3
./dist/amd64/civ.exe show 2458426 --json | python -c "import json,sys;json.load(sys.stdin)"
```

## 回滚点

- 纯文档里程碑，无代码回滚需求；spec 更新独立 commit