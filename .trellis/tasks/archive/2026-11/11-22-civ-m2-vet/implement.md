# M2 执行清单

- [ ] 1. internal/hashcache/cache.go：ComputeSHA256(path) (string, error) 流式实现 + GoDoc
  - 验证：hashcache_test.go（临时文件已知内容 → 期望 hex；与 sha256.Sum256 直算一致；空文件；不存在路径报错）
- [ ] 2. internal/cmd/vet.go：输入分类（classifyInput 纯函数）→ 路径算哈希 → api by-hash → 报告渲染 / --json
  - 验证：classifyInput 表驱动（64hex 大小写/10hex/路径/非法长度）
- [ ] 3. 报告渲染：复用 report 包，头块/触发词块/文件表/许可行/例图计数
  - 验证：vet_test.go（httptest 返回 byhash_anima_turbo fixture → 断言报告关键行；404 → exit 3；--json 可解析）
- [ ] 4. main.go 增 case "vet"；usage() 更新
- [ ] 5. 全量质量门：go vet / go test / go test -race
- [ ] 6. live 冒烟：已知 SHA256 vet exit 0 计时 <3s；AutoV2 命中；临时文件 exit 3；非法输入 exit 1
- [ ] 7. trellis-check → 验收核对 → commit 提醒（用户确认）

## 验证命令

```bash
go test ./internal/hashcache/ ./internal/cmd/
./dist/amd64/civ.exe vet FBA11953276B57EDF59D1DC4F1857AC05AA079C56F982B4D7C20298D57D3F7EB
./dist/amd64/civ.exe vet FBA1195327
echo test > /tmp/unknown.safetensors && ./dist/amd64/civ.exe vet /tmp/unknown.safetensors; echo "exit=$?"  # 期望 3
```

## 回滚点

- M1 完成 commit 为基线；M2 单 commit 交付