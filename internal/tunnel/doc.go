// Package tunnel 隧道握手协议与服务端连接处理。
//
// 关键文件：
//   handshake.go — JSON 握手字段与编解码
//   client_handshake.go — 客户端发起握手
//   server_handler.go — 鉴权、IP 分配、策略下发
//   source_ip.go — 隧道来源 IP 白名单（netutil.ParseHostIP）
//
// 上游：clientapp（出站）、serverapp（入站 Attach）。
// 下游：transport、crypto、sessionmgr、vpnaccount、netutil。
// 并发：每条 TLS 连接独立 Attach；ServerHandler 字段 Attach 后只读。
// 不变量：策略以服务端为准；密码登录成功才下发 client_private_key。
package tunnel
