// Package auth 处理 Web 登录鉴权：bcrypt 密码、Session Cookie、登录限流与锁定。
//
// 上游：api 包 Web 登录/会话校验；tunnel.ServerHandler 调用 VerifyTunnelLogin 做隧道密码鉴权。
// 下游：persist.Store 读写用户与密码哈希；golang.org/x/crypto/bcrypt 哈希校验。
// 并发：Service 内部 sessions 用 RWMutex、lockouts 用 Mutex；可并发 ValidateSession 与 Login。
// 不变量：隧道与 Web 共用同一用户库；连续失败达 maxAttempts 后按 clientIP 锁定 lockoutSec；
// 会话 token 仅存内存，进程重启后须重新登录。
package auth
