// Package transport TLS-TCP 自定义帧协议、连接生命周期与客户端自动重连。
//
// 关键文件：
//   frame.go — 帧常量、EncodeFrame、Decoder
//   transport.go — Conn、Dial、ListenTLS、Server
//   reconnect.go — ReconnectClient 指数退避
//   config_from.go — config → transport.Config 映射
//   pool.go — 读缓冲 sync.Pool
//
// 上游：clientapp、serverapp、tunnel（握手帧）。
// 下游：netutil 默认心跳/MTU 常量。
// 并发：每条 Conn 启动 read/write/heartbeat 三个 goroutine；须 Close 停止。
// 不变量：单帧 ≤ MaxFrameSize；Send 队列满时丢弃并返回 error。
package transport
