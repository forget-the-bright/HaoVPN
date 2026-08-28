# 安全加固清单

> 生产环境上线前逐项核对。默认配置已偏安全，本清单防止**交付时误配**。

---

## 1. 管理口暴露

| 检查项 | 要求 | 命令/方式 |
|--------|------|-----------|
| `api.allow_public_bind` | 必须为 `false` | 查看 `server.yaml` |
| `api.listen_hosts` | 不含 `0.0.0.0` / `::`（除非有充分理由且已评估） | 查看 `server.yaml` |
| frp / 防火墙 | **未**映射管理端口 8080 | 检查 frpc 配置 |
| 外网探测 | 公网 IP:8080 不可达 | `curl` 从外网测试 |

### 公开健康探针（有意设计）

`GET /api/v1/health` 与 `/api/v1/system/info` **无需登录**，用于就绪探针与版本定位。返回在线数、DB/TUN 状态等**非敏感**摘要；不包含密码、密钥或用户明细。若需隐藏，请在前置反代层限制来源 IP。

---

## 2. 账号与认证

| 检查项 | 要求 |
|--------|------|
| admin 默认密码 | 已修改，非模板初始值 |
| 密码强度 | ≥8 位，**须含字母与数字**（代码强制） |
| 闲置账号 | 禁用或删除 |
| 登录锁定 | `login_max_attempts` / `login_lockout_sec` 已配置 |
| `api.trusted_proxy_cidrs` | 生产默认**留空**；仅反代后且 RemoteAddr 命中信任 CIDR 时才解析 X-Forwarded-For（防锁定绕过） |
| `api.secure_cookies` | HTTPS 终止或全站 TLS 时设为 `true` |

---

## 3. TLS 与证书

| 检查项 | 要求 |
|--------|------|
| 生产证书 | 替换自签证书（`tls.auto_generate: false`） |
| 客户端校验 | `insecure_skip_verify: false` |
| 证书有效期 | 记录在案，到期前更换 |

---

## 4. 隧道与分流

| 检查项 | 要求 |
|--------|------|
| 分流 | `enforce_split_tunnel: true`，不下发 `0.0.0.0/0` |
| 工控网段 | `nat.allowed_lan_cidrs` 仅含必要网段 |
| 隧道来源 IP | 可选配置 `tunnel_allowed_source_ips` |

---

## 5. 文件与数据

| 检查项 | 要求 |
|--------|------|
| SQLite 权限 | Linux `chmod 600`；仅服务账户可读 |
| server.yaml 权限 | 限制为管理员可读 |
| 私钥/密码 | 日志与 `/api/v1/logs` 经 **Redact** 脱敏；仍禁止主动打印明文 |
| 定期备份 | 按 [deploy.md](deploy.md) 备份策略执行 |

---

## 6. 审计

| 检查项 | 要求 |
|--------|------|
| audit_logs | 启用且可查询 |
| 敏感操作 | 导出配置、踢人、改密均有记录 |

---

## 7. WebUI 与 CSP

| 检查项 | 说明 |
|--------|------|
| CSP `unsafe-inline` | **有意保留**：零构建链 HTML 模板需内联 script/style；不引外站 CDN。勿随意收紧 CSP，否则登录页白屏。 |

---

## 8. 系统

| 检查项 | 要求 |
|--------|------|
| 运行账户 | 非必要不用 root 登录；服务用专用账户 |
| 防火墙 | 仅开放必要端口（8443 隧道等） |
| 系统补丁 | 现场主机保持更新 |

---

## 9. 对外分发前（发版 / 交付）

| 检查项 | 要求 |
|--------|------|
| LICENSE / NOTICE | `dist/` 发版包须含根目录 [LICENSE](../LICENSE) 与 [NOTICE](../NOTICE)（`build-release` 自动复制） |
| 联系邮箱 | LICENSE §7 联系邮箱已由开发者填写（非占位符） |
| 版权头 | 禁止移除源码或二进制中的版权与许可声明 |
| 商用客户 | 须持书面授权后再商用部署 |

详见 [licensing.md](licensing.md)。

---

## 开发环境例外

开发联调可临时设置：

```yaml
api:
  listen_hosts: ["0.0.0.0"]
  allow_public_bind: true
```

**禁止**将此类配置用于生产。上线前运行：

```powershell
.\scripts\dev-security-check.ps1
```

---

*最后更新：2026-08-28 · 第十一轮*
