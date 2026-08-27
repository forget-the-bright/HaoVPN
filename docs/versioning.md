# 版本管理

## 唯一来源：`VERSION` 文件

项目根目录 [VERSION](../VERSION) 是**唯一的版本号来源**（语义化版本，如 `1.0.0`、`0.1.0-dev`）。

构建脚本、`-version` 输出、release 包均**读取此文件**，不得在脚本或代码中硬编码版本号。

## 谁可以改版本号

| 角色 | 是否可修改 `VERSION` |
|------|---------------------|
| **开发者（仓库所有者）** | ✅ 唯一有权修改 |
| **AI / 自动化助手** | ❌ **禁止修改** |

发版流程（仅开发者手动执行）：

1. 编辑根目录 `VERSION`（如 `0.1.0-dev` → `0.1.0`）
2. 运行 `.\scripts\build-release.ps1` 生成 `dist/`
3. 自行 `git commit`（见 [development-principles.md](development-principles.md) Git 规则）
4. 在 [dev-log.md](dev-log.md) 记录发版说明

## 构建时如何注入版本

```
go build -ldflags "-X main.version=<VERSION文件内容> -X main.commit=<git短hash> -X main.buildTime=<UTC时间>"
```

`commit` / `buildTime` 由构建脚本自动填充；**仅 `version` 来自 `VERSION` 文件**。

## AI 协作约束

- **不得**修改、提交、建议覆盖 `VERSION` 文件内容。
- **不得**在代码中将版本写死为常量（须读构建注入或运行时读 `VERSION`）。
- 若用户要求「发版」，AI 只应提示开发者**亲自**改 `VERSION` 并执行构建脚本。

---

*最后更新：2026-08-23*
