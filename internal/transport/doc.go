// Package transport 提供 TLS-TCP 自定义帧、连接生命周期与客户端自动重连。
//
// 关键文件（第十六轮同包拆分）：
//   config.go — Config、DefaultConfig、Effective*、AfterDisconnectPause
//   transport.go — Conn、Dial、AcceptConn、Send/Close/状态
//   conn_loops.go — readLoop / writeLoop / heartbeatLoop
//   server.go — Server、ListenTLS、acceptLoop、Close、Addr
//   mtu.go — ProbeMTU
//   frame.go / reconnect.go / config_from.go — 帧编解码、重连客户端、配置映射
//
// 上游：clientapp、serverapp、tunnel（握手帧）。
// 下游：netutil（默认心跳/MTU/队列）、timeutil、config。
// 并发：每条 Conn 独立读写心跳 goroutine；须 Close 停止。
// 不变量：单帧 ≤ MaxFrameSize；Send 队列满返回 error（不静默丢关键控制帧语义由调用方保证）。
package transport
