// Package transport 提供 TLS-TCP 自定义帧、连接生命周期与客户端自动重连。
//
// 关键文件（同包拆分；拨号哨兵在 dialerr）：
//   config.go — Config、DefaultConfig、Effective*、AfterDisconnectPause
//   transport.go — Conn、Dial、AcceptConn、Send/Close/状态（读写心跳经 safeutil.GoSafe）
//   conn_loops.go — readLoop / writeLoop / heartbeatLoop
//   server.go — Server、ListenTLS、acceptLoop（GoSafe；TLS 失败上报 Probe）；拒绝时 WriteRejectBanner
//   probe_banner.go — TLS 前 banner I/O（peek/写出）；哨兵与常量见 dialerr
//   mtu.go — ProbeMTU
//   frame.go / reconnect.go / config_from.go — 帧编解码、重连客户端（GoSafe/Done/ExpBackoff）、配置映射
//
// TLS 前拒绝协议（客户端必读）：
//  1. 服务端 CheckAccept 拒绝 → 先 WriteRejectBanner 再 Close（记库异步，勿阻塞写出）
//  2. 客户端 Dial 后短 peek（约 250ms）识别 banner；成功路径服务端此时无字节，故 peek 必须短
//  3. 明确 banner → dialerr.ErrIPBanned / ErrSourceDenied；无 banner 的 EOF → ErrClosedBeforeTLS（可重试）
//  4. peek 过后 TLS 读到明文 → dialerr.ErrPlaintextBeforeTLS（致命停重连，文案双因：封禁或连错端口）
//
// 上游：clientapp、serverapp、tunnel（握手帧）。
// 下游：dialerr（拨号哨兵）、safeutil、netutil、timeutil、config、logger。
// 并发：每条 Conn 独立读写心跳 goroutine（GoSafe）；须 Close 停止。
// 不变量：单帧 ≤ MaxFrameSize；Send 队列满返回 error（不静默丢关键控制帧语义由调用方保证）。
package transport
