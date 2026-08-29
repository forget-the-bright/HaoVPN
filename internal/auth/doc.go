// Package auth 处理 Web 登录鉴权与隧道密码校验：bcrypt、Session/CSRF、登录限流。
//
// 关键文件（同包拆分，降低阅读耦合；导出 API 不变）：
//   errors.go — 登录/握手哨兵（ErrBadCredentials、ErrLoginLocked 等），供 tunnel/clientapp errors.Is
//   service.go — Service、SessionEntry、New；webLockouts 与 tunnelLockouts 分表
//   password_ops.go — MustChangePassword、ChangePassword(须旧密码)、ResetPasswordByAdmin、UserActiveForSession
//   login.go — EnsureAdmin、Web Login、verifyCredentials（禁用/非 admin 对外模糊为错密）
//   tunnel_login.go — VerifyTunnelLogin（隧道专用，不创建 Web 会话）
//   session.go — ValidateSession、Logout、LogoutAllForUser、PruneExpiredSessions、CSRF（常量时间比较）
//   lockout.go — 按 realm+clientIP 的失败累计与锁定
//
// 上游：api 包 Web 登录/会话校验；tunnel.ServerHandler 调用 VerifyTunnelLogin。
// 下游：persist.Store；golang.org/x/crypto/bcrypt。
// 并发：sessions 用 RWMutex、lockouts 用 Mutex。
// 不变量：Web 与隧道锁定表隔离；连续失败达 maxAttempts 后按 IP 锁定；
// 会话 token 仅存内存；改密/重置/禁用后须 LogoutAllForUser；致命握手错误用哨兵。
package auth
