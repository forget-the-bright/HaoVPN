// Package transport 提供 TLS-TCP 自定义帧、连接生命周期与客户端自动重连。
//
// 关键文件（第十六轮同包拆分）：
//   config.go — Config、DefaultConfig、Effective*、AfterDisconnectPause
//   transport.go — Conn、Dial、AcceptConn、Send/Close/状态
//   conn_loops.go — readLoop / writeLoop / heartbeatLoop
//   server.go — Server、ListenTLS、acceptLoop（TLS 握手失败上报 Probe）；拒绝时 WriteRejectBanner
//   probe_banner.go — TLS 前拒绝码 HAOVPN:IP_BANNED / SOURCE_DENIED；短 peek；FatalDial 判定
//   mtu.go — ProbeMTU
//   frame.go / reconnect.go / config_from.go — 帧编解码、重连客户端、配置映射
//
// TLS 前拒绝协议（客户端必读）：
//  1. 服务端 CheckAccept 拒绝 → 先 WriteRejectBanner 再 Close（记库异步，勿阻塞写出）
//  2. 客户端 Dial 后短 peek（约 250ms）识别 banner；成功路径服务端此时无字节，故 peek 必须短
//  3. 明确 banner → ErrIPBanned / ErrSourceDenied；无 banner 的 EOF → ErrClosedBeforeTLS（可重试）
//  4. peek 过后 TLS 读到明文 → ErrPlaintextBeforeTLS（致命停重连，文案双因：封禁或连错端口）
//
// 上游：clientapp、serverapp、tunnel（握手帧）。
// 下游：netutil（默认心跳/MTU/队列）、timeutil、config。
// 并发：每条 Conn 独立读写心跳 goroutine；须 Close 停止。
// 不变量：单帧 ≤ MaxFrameSize；Send 队列满返回 error（不静默丢关键控制帧语义由调用方保证）。
package transport
