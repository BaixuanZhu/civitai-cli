# M3 sync/login/--prompts 与哈希缓存

## Goal

交付批量对账 `civ sync`、token 管理 `civ login/logout`、例图配方 `--prompts`（show/vet）。完成后规划文档全部功能命令就绪。

## Requirements

### sync（规划文档 §4.4）

- 递归扫描目录，--ext 默认 `.safetensors,.pt,.pth,.ckpt,.bin`（逗号分隔可覆盖）
- 哈希缓存：%LocalAppData%\civ\cache\hashes.json，键 absPath → {size, mtime, sha256}；size/mtime 任一变化重算；--rehash 全量；CIV_NO_CACHE 非空绕过缓存（读写都跳过）
- 文件哈希并发默认 4；API 并发默认 3（--concurrency，上限 8，复用 client 并发闸）
- 输出行：FILE | STATUS(found/not_found) | MODEL | VERSION | BASE | TRIGGERS(n) | PICKLE | VIRUS | MODEL_URL
- 同一版本命中多个本地文件 → 标 dupe
- --format table|csv|json（默认 table）；全局 --json 等价 --format json；CSV 用 encoding/csv
- 退出码：全部 API 调用成功即 0（not_found 是数据行）；网络重试耗尽 2；用法 1
- 进度走 stderr（[INFO] n/total），不污染 stdout 数据

### login/logout（规划文档 §4.6）

- login：交互粘贴 key（stderr 提示 + stdin 读取；非 TTY 也支持管道输入）→ favorites 探针 GET /models?favorites=true&limit=1（200 有效 / 401 无效 exit 4）→ 写 os.UserConfigDir()/civ/config.json
- logout：删除配置文件（不存在则友好提示，exit 0）
- token 优先级补全：--token > CIVITAI_API_KEY > config.json（internal/config 包，M1 的解析点插入第三层）
- config 写入 0600 语义 Windows 尽力而为（os.WriteFile 0600）

### --prompts（规划文档 §4.2，show 与 vet 都支持）

- 取版本详情 images[].meta，按例图编号输出 prompt/negativePrompt/steps/cfgScale/sampler/scheduler
- meta 为 null 的例图跳过并计数（"skipped N images without meta"）
- 纯文本块每行一条可复制；--json 时结构化数组
- 匿名上限说明：版本详情 images 匿名全量（规划文档 §3.3-4），无需 token

## Acceptance Criteria

- [ ] 合成目录（50 个临时文件）sync 出 CSV/JSON/table 三形态；not_found 行正确；dupe 标记正确（httptest 模拟同版本多文件命中）
- [ ] 二次运行哈希缓存命中（文件哈希不重算——计数断言）；touch 改 mtime 后重算；--rehash 全量重算
- [ ] CIV_NO_CACHE=1 时缓存文件不读不写
- [ ] login 探针 200 → config 写入且后续命令自动带 token；401 → exit 4 不写 config；logout 删除
- [ ] token 优先级三层单测（flag > env > config）
- [ ] show -v <id> --prompts 输出配方块（fixture version_3263843）；null meta 跳过计数
- [ ] 性能：50 文件 ≤40s、缓存二次 ≤5s（真实目录可用时 live 实测；否则合成基准 + 文档注明）
- [ ] go vet / go test / go test -race 全绿零外网

## Notes

- 技术设计见 design.md；全局契约父 design.md D8
- 真实 LoRA 目录路径需向用户要（open question，规划文档示例 I:\ComfyUI-Workspace\models\loras）