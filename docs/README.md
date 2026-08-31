# 文档索引

> **单一职责**：本页只做导航与放置规则。进度看 [dev-log.md](dev-log.md)；包结构看 [architecture.md](architecture.md)；版本看根目录 [`VERSION`](../VERSION)（发版说明写 **dev-log** / 可选 GitHub Release，**不**在 docs 堆草稿）。

接手请先读根目录 [记忆.md](../记忆.md) 与 [README.md](../README.md)。

---

## 放置规则（新增文档必读）

| 类型 | 放哪 |
|------|------|
| 现行必读、运维手册、CODEMAP、进度日志 | `docs/` **根目录**（路径稳定，代码注释常硬引用） |
| 未开工方案、专题蓝图 | [`plans/`](plans/) |
| 过时规划、审计快照、已关闭草案 | [`archive/`](archive/) |
| 发版说明 | **禁止**新增 `release-notes-*-DRAFT.md`；改 `VERSION` + [dev-log](dev-log.md) +（可选）GitHub Release |

---

## 必读（开发）

| 文档 | 说明 |
|------|------|
| [codebase-guide.md](codebase-guide.md) | **代码库导读**（分层、横切关注点、新人全景） |
| [development-principles.md](development-principles.md) | 开发原则与验收底线 |
| [architecture.md](architecture.md) | **CODEMAP**（包边界、依赖、HTTP 路由） |
| [../internal/README.md](../internal/README.md) | 改 X 功能来哪 |
| [comment-style.md](comment-style.md) | 中文注释规范 |
| [dev-log.md](dev-log.md) | **唯一进度 / 变更日志** |
| [versioning.md](versioning.md) | 版本管理（**AI 禁止改 `VERSION`**） |

## 部署与运维

| 文档 | 说明 |
|------|------|
| [deploy.md](deploy.md) | 部署拓扑、配置、验收、客户端自启 |
| [troubleshooting.md](troubleshooting.md) | 现场排障 |
| [security-hardening.md](security-hardening.md) | 生产加固清单 |
| [../scripts/README.md](../scripts/README.md) | 构建 / 验收脚本 |

## 设计专题

| 文档 | 说明 |
|------|------|
| [traffic-routing.md](traffic-routing.md) | 流量/路由走向：分流 vs 托管、代码路径、与 OpenVPN 对照 |

## 产品与法律

| 文档 | 说明 |
|------|------|
| [licensing.md](licensing.md) | 商用授权中文说明 |
| [../cmd/README.md](../cmd/README.md) | 三个二进制入口 |
| [../web/README.md](../web/README.md) | WebUI 模板与 `static/*.js` |

## 规划（未开工）

| 文档 | 说明 |
|------|------|
| [plans/mobile-client-plan.md](plans/mobile-client-plan.md) | 手机客户端实施蓝图（代码未开工） |

## 归档（勿当现行手册）

| 文档 | 说明 |
|------|------|
| [archive/meta-plan.md](archive/meta-plan.md) | v1.0 规划存档（目录树可能过时；结构以 architecture 为准） |
| [archive/code-audit.md](archive/code-audit.md) | 历史代码审计（勿当现行缺陷清单） |

---

## 文档维护约定

1. **进度只写 [dev-log.md](dev-log.md)**，不要在本索引或记忆.md 堆「第 N 轮摘要」。
2. **包导航只维护 architecture + internal/README**，其它文档链过去，避免抄两份 CODEMAP。
3. 改行为（配置项、安全默认、自启、API）时同步：`deploy` / `security-hardening` / `troubleshooting` 中相关节。
4. 新增文档遵守上方「放置规则」；活文档尽量留在根目录，避免打断代码里的 `docs/deploy.md` 等硬路径。

*最后更新：2026-08-31 · 文档治理（删 release-notes DRAFT；plans/archive 浅分层）*
