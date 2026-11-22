# 执行清单 — civ skill 子命令 + civ-prompt Skill

> 顺序：Skill 内容 → internal/skill 包 → cmd 接线 → 测试 → 文档/冒烟。纯本地功能，无外网依赖（除真实平台可见性冒烟）。

## Skill 内容

- [ ] 1. internal/skill/assets/civ-prompt/SKILL.md（标准六字段 frontmatter，metadata.version 占位 __CIV_VERSION__；body <500 行 imperative，按 design §6）
- [ ] 2. references/civ-commands.md（命令+--json 形状+退出码速查）、references/prompt-patterns.md（触发词/配方组装模式与陷阱）
  - 验证：SKILL.md 校验（name=civ-prompt、description 单行≤1024 无尖括号、body<500 行、仅六字段、无 TODO）

## internal/skill 包

- [ ] 3. embed.go（//go:embed assets/civ-prompt → FS；SkillName 常量）+ skill.go（Platform/Scope/RootFor/DetectPlatforms/版本渲染/极简 frontmatter version 提取）
  - 验证：RootFor 表驱动（三平台×两 scope，codex→.agents 非 .codex）；DetectPlatforms 临时 HOME；版本渲染
- [ ] 4. install.go（Install/Remove/Update + ConflictFunc 回调 + Result；mkdir -p + embed walk 写入 + backup；不动 zcode lock）
  - 验证：临时 HOME install/remove/update（版本 skip/overwrite/backup/冲突回调）；embed FS 含 SKILL.md+references

## cmd 接线

- [ ] 5. internal/cmd/skill.go（install/remove/update 动作解析 + --platform/--scope/--force flag + 交互 stdin 参数化 + --json []Result + 退出码 0/1）
  - 验证：cmd 测试（flag 模式装到临时根/交互 stdin 注入/无效平台 scope→1/.remove/update/--json）
- [ ] 6. main.go 增 case "skill"；usage() 增 skill 行
  - 验证：make build；civ skill --help / civ skill install --platform claude --scope user（临时 HOME 冒烟）

## 收尾

- [ ] 7. 全量质量门：go vet ./... && go test ./... && go test -race ./...；gofmt -l . 空
- [ ] 8. 嵌入验证：make build 后，移开仓库源（或临时 HOME 空目录）civ skill install 仍能从二进制装出 Skill（证明 embed 自携带）
- [ ] 9. 真实平台冒烟：claude/zcode 本机 install→检查 ~/.claude/skills/civ-prompt/ 与 ~/.zcode/skills/civ-prompt/ 文件齐全 + metadata.version 正确；agent 可见性（/skills list 或目录扫描）；codex 本机未装注明留 CI
- [ ] 10. README skill 章节（install/remove/update + 三平台 + 交互/flag + civ-prompt 用途）；CHANGELOG ### Added 条目
- [ ] 11. trellis-check 质量审查 → prd 验收核对 → commit（用户确认）

## 验证命令

```bash
go vet ./... && go test ./... && go test -race ./...
make build
# 临时 HOME 隔离冒烟（不污染真实平台目录）：
HOME=/tmp/civskilltest USERPROFILE=/tmp/civskilltest ./dist/amd64/civ.exe skill install --platform claude,zcode,codex --scope user
ls /tmp/civskilltest/.claude/skills/civ-prompt/ /tmp/civskilltest/.zcode/skills/civ-prompt/ /tmp/civskilltest/.agents/skills/civ-prompt/
HOME=/tmp/civskilltest ./dist/amd64/civ.exe skill update --platform claude   # up-to-date
HOME=/tmp/civskilltest ./dist/amd64/civ.exe skill remove --platform claude
```

## 回滚点

- 基线 commit（c4f359e）为基线；Skill 内容 / internal/skill / cmd 接线可分 commit；失败 revert 到上一 commit
- skill 安装本身可逆（civ skill remove）