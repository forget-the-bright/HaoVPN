# 文档索引

> **单一职责**：本页只做导航。进度看 [dev-log.md](dev-log.md)；包结构看 [architecture.md](architecture.md)；发版看根目录 [`VERSION`](../VERSION) 与 release-notes 草稿。

接手请先读根目录 [记忆.md](../记忆.md) 与 [README.md](../README.md)。

---

## 必读（开发）

| 文档 | 说明 |
|------|------|
| [development-principles.md](development-principles.md) | 开发原则与验收底线 |
| [architecture.md](architecture.md) | **CODEMAP**（包边界、依赖、HTTP 路由） |
| [../internal/README.md](../internal/README.md) | 改 X 功能来哪 |
| [comment-style.md](comment-style.md) | 中文注释规范 |
| [dev-log.md](dev-log.md) | **唯一进度 / 变更日志** |
| [versioning.md](versioning.md) | 版本管理（AI 禁止改 `VERSION`） |

## 部署与运维

| 文档 | 说明 |
|------|------|
| [deploy.md](deploy.md) | 部署拓扑、配置、验收、客户端自启 |
| [troubleshooting.md](troubleshooting.md) | 现场排障 |
| [security-hardening.md](security-hardening.md) | 生产加固清单 |
| [../scripts/README.md](../scripts/README.md) | 构建 / 验收脚本 |

## 产品与法律

| 文档 | 说明 |
|------|------|
| [licensing.md](licensing.md) | 商用授权中文说明 |
| [meta-plan.md](meta-plan.md) | v1.0 规划**存档**（目录树可能过时；结构以 architecture 为准） |
| [../cmd/README.md](../cmd/README.md) | 三个二进制入口 |
| [../web/README.md](../web/README.md) | WebUI 模板与 `static/*.js` |

## 手机客户端（尚未开工代码）

| 文档 | 说明 |
|------|------|
| [mobile-client-plan.md](mobile-client-plan.md) | **实施蓝图**：架构、步骤、缺陷清单、安全、验收；按此即可开工 |

## 发版说明

| 文档 | 说明 |
|------|------|
| [release-notes-0.1.2-DRAFT.md](release-notes-0.1.2-DRAFT.md) | **当前草稿**：0.1.1 → 0.1.2 |
| [release-notes-0.1.1-DRAFT.md](release-notes-0.1.1-DRAFT.md) | 历史：0.1.0 → 0.1.1 |

## 归档

| 文档 | 说明 |
|------|------|
| [archive/code-audit.md](archive/code-audit.md) | 历史代码审计（勿当现行缺陷清单） |

---

## 文档维护约定

1. **进度只写 dev-log**，不要在本索引或记忆.md 堆「第 N 轮摘要」。
2. **包导航只维护 architecture + internal/README**，其它文档链过去，避免抄两份 CODEMAP。
3. 改行为（配置项、安全默认、自启、API）时同步：`deploy` / `security-hardening` / `troubleshooting` 中相关节。
4. 文件名与链接保持一致：`development-principles`、`comment-style`、`licensing`、`deploy`、`dev-log`。

*最后更新：2026-08-30 · 增补 mobile-client-plan*
