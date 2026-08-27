package netutil

// DefaultMTU 全项目统一的隧道/TUN 默认 MTU（字节）。
//
// 与 client.yaml / server.yaml 模板及 ApplyDefaults 一致；握手 policy 可覆盖。
const DefaultMTU = 1420

// 传输层、重连与审计保留的秒级/天级默认值（config.ApplyDefaults 与 GUI 内存配置共用）。
const (
	// DefaultHeartbeatIntervalSec 客户端/服务端心跳发送间隔（秒）。
	DefaultHeartbeatIntervalSec = 15
	// DefaultHeartbeatTimeoutSec 对端静默超过此秒数判定断线（秒）。
	DefaultHeartbeatTimeoutSec = 90
	// DefaultDialTimeoutSec TCP 拨号至 server.address 的超时（秒）。
	DefaultDialTimeoutSec = 3
	// DefaultReconnectInitialSec 断线后首次重连等待（秒）。
	DefaultReconnectInitialSec = 1
	// DefaultReconnectMaxSec 指数退避重连间隔上限（秒）。
	DefaultReconnectMaxSec = 3
	// DefaultRetentionDays 审计日志、连接事件与 history 库默认保留天数。
	DefaultRetentionDays = 90
)

// TunReadBufferExtra TUN Read 循环在 MTU 之外额外预留的字节数。
//
// 用于以太网头/对齐余量，ReadBufferSize 返回 MTU + 本常量。
const TunReadBufferExtra = 100
