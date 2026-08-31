// Package tunnel 隧道握手协议与服务端连接处理。
//
// 关键文件：
//   handshake.go — JSON 握手字段与编解码（含 handshake_err.code）
//   client_handshake.go — 客户端发起握手；FromHandshakeCode 还原哨兵
//   server_handler.go — ServerHandler / Attach / doHandshake 编排 / LAN 注册
//   server_handshake_auth.go — 握手阶段 1～3（源 IP、鉴权、私钥解密）
//   server_handshake_session.go — 握手阶段 4～7（IP、策略、注册、OK、转发）
//   handshake_reject.go — 握手拒绝（code+文案）+ ProbeRecorder.OnHandshakeReject
//
// 上游：clientapp（出站）、serverapp（入站 Attach）。
// 下游：transport、crypto、sessionmgr、vpnaccount、auth、autherr、netutil、dialerr（经 autherr）。
// 禁止 import probedefense（探针经 ProbeRecorder 窄接口）。
// 并发：每条 TLS 连接独立 Attach；ServerHandler 字段 Attach 后只读。
// 不变量：
//   - 策略以服务端为准；密码登录成功才下发 client_private_key；
//   - 默认拒绝库内明文私钥（AllowPlaintextPrivateKeys 仅兼容旧库）；
//   - handshake_err 带稳定 code；客户端 FromHandshakeCode 还原哨兵；
//   - 源 IP 白名单直接调 netutil.CheckSourceIPAllowed（无本包薄包装）。
package tunnel
