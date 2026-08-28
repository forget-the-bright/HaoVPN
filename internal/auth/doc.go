// Package auth 处理 Web 登录鉴权与隧道密码校验：bcrypt、Session/CSRF、登录限流。
//
// 关键文件（同包拆分，降低阅读耦合；导出 API 不变）：
//   service.go — Service、SessionEntry、New
//   password.go — HashPassword / CheckPassword
//   login.go — EnsureAdmin、Web Login、verifyCredentials
//   tunnel_login.go — VerifyTunnelLogin（隧道专用，不创建 Web 会话）
//   session.go — ValidateSession、Logout、CSRF、createSession
//   lockout.go — 按 clientIP 的失败累计与锁定
//
// 上游：api 包 Web 登录/会话校验；tunnel.ServerHandler 调用 VerifyTunnelLogin。
// 下游：persist.Store；golang.org/x/crypto/bcrypt。
// 并发：sessions 用 RWMutex、lockouts 用 Mutex。
// 不变量：隧道与 Web 共用同一用户库；连续失败达 maxAttempts 后按 IP 锁定；
// 会话 token 仅存内存，进程重启后须重新登录。
package auth
