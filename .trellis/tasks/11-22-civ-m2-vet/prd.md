# M2 vet 审卡命令

## Goal

交付旗舰命令 `civ vet <path|sha256|autov2>`：本地文件反查 Civitai 登记信息，输出审查报告。设计契约见父 design.md §3（vet 数据流）与规划文档 §4.3。

## Requirements

- 输入分类：64 位十六进制（大小写不敏感）→ SHA256；10 位十六进制 → AutoV2；其余视为本地文件路径（存在性校验，不存在 exit 1）
- 流式哈希：internal/hashcache.ComputeSHA256(path)——io.Copy 到 sha256.New，32KB buffer，不整文件读入内存；输出大写 hex
- by-hash 反查：复用 M1 api 端点（url.PathEscape）；404 → NotFoundError → exit 3，stderr 提示"可能非 Civitai 来源或文件被改"
- 报告渲染（规划文档 §4.3 样例格式）：MODEL/VERSION/CREATOR 头块、TRIGGER WORDS 纯文本块、FILES（名称/大小/format/fp/primary/全量哈希 SHA256+AutoV2/scan pickle+virus）、LICENSE 四件套、EXAMPLES 计数（mature 计数，--prompts 提示留 M3）
- --json：结构化审查报告对象（version + model 摘要 + files + license + examples 计数）
- 退出码：0 命中 / 3 未收录 / 1 用法 / 2 网络 / 4 token 失效
- --prompts flag 本里程碑先注册但提示 M3 实现？不——M2 不注册 --prompts，M3 统一加（避免半成品 flag）

## Acceptance Criteria

- [ ] 已知哈希 vet 出完整报告 exit 0（附录 D Anima SHA256：FBA11953...F7EB，live 冒烟）
- [ ] 本地临时文件（任意内容）vet → 流式哈希 → 404 → exit 3
- [ ] 直接给 AutoV2（FBA1195327）vet 命中同一版本
- [ ] 不存在路径 / 非法哈希长度 → exit 1
- [ ] 单测：输入分类表驱动、ComputeSHA256 与 crypto/sha256 直算一致、报告渲染 fixture 断言、404→3 映射（httptest）
- [ ] `go test ./...` 零外网全绿；vet 单文件 < 3s（live 冒烟计时）

## Notes

- 轻量任务：PRD + implement 清单即可，技术设计继承父 design.md（D2/D3/D4/D7 + §3 数据流）
- hashcache 包本里程碑只建 ComputeSHA256；缓存读写 M3 扩展