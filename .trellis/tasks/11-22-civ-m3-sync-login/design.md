# M3 技术设计 — sync/login/--prompts

> 全局契约见父 design.md（D8 配置与缓存路径）。本文补 M3 细节。

## 1. internal/hashcache 扩展

```go
type Entry struct { Size int64; MTime time.Time; SHA256 string }
type Cache struct { path string; entries map[string]Entry; dirty bool }
func Load() *Cache            // os.UserCacheDir()/civ/cache/hashes.json；损坏 → [WARN] 重建（降级不致命）
func (c *Cache) Get(path string, fi fs.FileInfo) (string, bool)  // size+mtime 匹配才命中
func (c *Cache) Put(path string, fi fs.FileInfo, hash string)
func (c *Cache) Save() error  // 原子写：同目录 temp + rename；dirty 才写
func ComputeSHA256(path string) (string, error)  // M2 已有
```

- CIV_NO_CACHE 非空：Load 返回空缓存且 Save 跳过（cmd 层判断，包内不读 env——保持叶子包纯净，由 cmd 传入 disabled 标志或 nil cache）
- 决策：cmd/sync 持有 *Cache（nil = 禁用），hashcache 包不感知 env；Get/Put 对 nil receiver 安全（no-op）
- mtime 比较用 fi.ModTime().Equal(entry.MTime)（JSON 往返 time.Time 精度 RFC3339Nano 保真）

## 2. sync 并发模型

```
WalkDir 收集文件列表（--ext 过滤，跳过目录错误单文件 [WARN] 继续）
  → 阶段1：文件哈希 worker pool（4 goroutine + jobs channel）
      每文件：cache.Get 命中跳过 / ComputeSHA256 → cache.Put
      结果 []fileHash{path, hash, err}；哈希失败行 STATUS=error（不阻断整体）
  → cache.Save()（阶段1 结束即落盘，API 失败也不丢哈希劳动）
  → 阶段2：API worker pool（--concurrency goroutine，client.sem 双保险）
      每文件：api.ByHash → found/not_found/err 分类
  → 聚合：versionID → []path，多文件标 dupe
  → 渲染 table/csv/json
```

- 两阶段分离而非流水线：哈希快（本地 IO）API 慢（网络），阶段1 全量完成后缓存先落盘，阶段2 失败重跑不重算哈希——满足"二次 ≤5s"验收
- ctx 取消（Ctrl+C）：worker select ctx.Done，已算哈希仍 Save
- API 错误行：NetworkError → STATUS=error + stderr [WARN]，整体 exit 2（任一网络失败）；404 → not_found 行 exit 0

## 3. internal/config

```go
type Config struct { Token string `json:"token"` }
func Path() string                 // os.UserConfigDir()/civ/config.json
func Load() (Config, error)        // 不存在 → 零值 nil error
func Save(c Config) error          // MkdirAll 0700 + WriteFile 0600
func Delete() error                // logout；不存在 → nil
func ResolveToken(flagToken string) string  // flag > CIVITAI_API_KEY > Load().Token
```

- M1 的 token 解析点改为调 config.ResolveToken（cmd 全局 flag helper 内）
- login 探针走 api client（带候选 token）：200 → Save；401 → exit 4；网络错 → exit 2
- login 交互：fmt.Fprint(os.Stderr, "Paste API key: ") + bufio.Scanner(os.Stdin)；读到的 key TrimSpace；空输入 exit 1

## 4. --prompts 渲染（internal/cmd/prompts.go 共享 helper）

```go
func renderPrompts(w io.Writer, images []api.Image, jsonMode bool)
```

- show -v 与 vet 共用：遍历 images，Meta==nil 跳过计数；编号 [1] [2]…；字段顺序 prompt/negativePrompt/steps/cfgScale/sampler/scheduler（缺省字段跳过）
- prompt 多行原文保留（不压行）；--json 输出 [{index, url, prompt, ...}] + skipped 计数
- vet 报告 EXAMPLES 行后追加提示 "--prompts 查看例图配方"（M2 已留）

## 5. 测试要点

- hashcache：临时目录 + 可控 mtime（os.Chtimes）表驱动失效判定；损坏 JSON 降级；原子写（Save 后文件存在且可 Load）；nil receiver 安全
- sync：httptest by-hash 路由（按 hash 返回不同 fixture / 404）+ 临时目录合成文件；断言 CSV 行/dupe/缓存命中计数（第二次跑 ComputeSHA256 调用数 = 0，用注入计数或文件 atime 不可靠——改断言 cache.Get 命中路径：包内测试直接验证 Get/Put 语义 + cmd 测试断言二次运行耗时/输出一致）
- config：三层优先级表驱动（t.Setenv）；Save 权限位断言（Windows 尽力而为，不断言 0600 实际 ACL）
- login：httptest 200/401；stdin 用 strings.Reader 注入（cmd 函数接受 io.Reader 参数化 stdin 便于测试）
- prompts：fixture null meta 跳过计数；字段缺省跳过