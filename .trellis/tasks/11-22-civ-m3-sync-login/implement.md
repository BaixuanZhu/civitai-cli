# M3 执行清单

- [ ] 1. internal/config/config.go：Path/Load/Save/Delete/ResolveToken + 测试（优先级表驱动 t.Setenv）
- [ ] 2. M1 token 解析点接入 config.ResolveToken；全局 flag helper 更新
  - 验证：go test ./... 仍全绿
- [ ] 3. internal/hashcache 扩展：Entry/Cache/Load/Get/Put/Save（原子写、nil 安全、损坏降级）
  - 验证：hashcache_test.go（mtime/size 失效表驱动、原子写往返、损坏 JSON 降级、nil receiver）
- [ ] 4. internal/cmd/login.go：login（stdin 参数化 + 探针 + Save）/ logout（Delete）；main.go 增 case
  - 验证：login_test.go（httptest 200 写 config / 401 exit 4 不写 / 空输入 exit 1 / logout 幂等）
- [ ] 5. internal/cmd/prompts.go：renderPrompts 共享 helper；show -v 与 vet 接 --prompts flag
  - 验证：prompts_test.go（fixture 配方块 / null meta 跳过计数 / --json 结构）
- [ ] 6. internal/cmd/sync.go：WalkDir + 两阶段 worker pool + dupe 聚合 + table/csv/json 渲染 + 进度 stderr
  - 验证：sync_test.go（httptest 多 hash 路由 + 临时目录：found/not_found/dupe/三形态输出/缓存二次命中/CIV_NO_CACHE 绕过/--rehash）
- [ ] 7. main.go 增 case "sync"；usage() 更新；--concurrency 上限 8 校验
- [ ] 8. 全量质量门：go vet / go test / go test -race
- [ ] 9. 性能验收：向用户要真实 LoRA 目录（或合成 50 文件基准）；计时 sync 首跑 ≤40s / 二次 ≤5s，结果记入 prd 验收备注
- [ ] 10. trellis-check → 验收核对 → commit 提醒（用户确认）

## 验证命令

```bash
go test ./internal/config/ ./internal/hashcache/ ./internal/cmd/
make build
mkdir -p /tmp/synctest && for i in $(seq 1 50); echo "dummy $i" > /tmp/synctest/f$i.safetensors; done
time ./dist/amd64/civ.exe sync /tmp/synctest --format csv > /tmp/audit.csv   # 首跑（全 not_found，验证管线）
time ./dist/amd64/civ.exe sync /tmp/synctest --format csv > /tmp/audit2.csv  # 二次（缓存命中）
CIV_NO_CACHE=1 ./dist/amd64/civ.exe sync /tmp/synctest --rehash --limit 2>/dev/null
```

## 回滚点

- M2 commit 为基线；config/hashcache/login/prompts/sync 可分 commit，sync 失败不影响已交付 login/prompts