package netutil

// DefaultMTU 全项目统一的隧道/TUN 默认 MTU（字节）。
//
// 与 client.yaml / server.yaml 模板及 ApplyDefaults 一致；握手 policy 可覆盖。
const DefaultMTU = 1420

// 传输层与重连的秒级默认值（config.ApplyDefaults 与 GUI 内存配置共用）。
// 保留天数默认见 config.DefaultRetentionDays（非网络常量，勿放入本包）。
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
)

// TunReadBufferExtra TUN Read 循环在 MTU 之外额外预留的字节数。
//
// 用于以太网头/对齐余量，ReadBufferSize 返回 MTU + 本常量。
const TunReadBufferExtra = 100

// 传输发送队列深度（待发帧条数，非字节）。满则丢帧并 WARN。
const (
	// DefaultSendQueueSize 默认待发帧队列深度（与 transport.DefaultConfig 一致）。
	DefaultSendQueueSize = 256
	// MinSendQueueSize YAML 允许的最小队列（过小易打满）。
	MinSendQueueSize = 64
	// MaxSendQueueSize YAML 允许的最大队列（过大增延迟与内存）。
	MaxSendQueueSize = 8192
)

// ClampSendQueueSize 将队列深度钳到 [Min, Max]；≤0 视为默认。
//
// 返回：钳制后的值，以及是否发生了钳制（调用方可打 Warn）。
func ClampSendQueueSize(n int) (clamped int, changed bool) {
	if n <= 0 {
		return DefaultSendQueueSize, n != 0
	}
	if n < MinSendQueueSize {
		return MinSendQueueSize, true
	}
	if n > MaxSendQueueSize {
		return MaxSendQueueSize, true
	}
	return n, false
}
