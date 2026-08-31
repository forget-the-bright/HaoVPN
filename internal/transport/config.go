package transport

import (
	"time"

	"haovpn/internal/netutil"
	"haovpn/internal/timeutil"
)

// Config 传输层心跳、队列、重连与 MTU 参数（由 client/server YAML 经 config_from 映射）。
//
// 字段：
//   HeartbeatInterval — 发送心跳帧间隔；0 时 DefaultConfig 用 netutil 默认。
//   HeartbeatTimeout — 读超时与对端静默判定；超时则 Close。
//   WriteTimeout — 单次 TLS Write 截止时间；队列阻塞另受 MaxQueueSize 限制。
//   MaxQueueSize — 待发帧 channel 容量；满时 SendRaw 丢弃并返回错误。
//   ReconnectInitial — 重连首次退避；0 时 EffectiveReconnectInitial 为 1s。
//   ReconnectMax — 退避上限；0 时 EffectiveReconnectMax 为 3s。
//   DialTimeout — TCP 拨号超时；0 时 EffectiveDialTimeout 为 3s（避免损耗链路空等）。
//   MTU — 读缓冲与帧大小参考；与 VPN 内层 MTU 对齐，默认 netutil.DefaultMTU。
//   Probe — 可选探针防御观测（服务端注入）；客户端保持 nil。
type Config struct {
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
	WriteTimeout      time.Duration
	MaxQueueSize      int
	ReconnectInitial  time.Duration
	ReconnectMax      time.Duration
	DialTimeout       time.Duration // TCP 拨号超时；0 表示用默认 3s（损耗链路避免空等 10s）
	MTU               int
	Probe             ProbeObserver // 服务端可选；Accept/读错误/拆帧失败时回调
}

// ProbeObserver 服务端探针防御钩子（由 probedefense.Guard 适配）。
//
// CheckAccept 在 TCP 接入后、TLS 握手前调用；rejectBanner 非空时须先写入客户端再关闭。
type ProbeObserver interface {
	AllowAccept(remoteAddr string) bool
	CheckAccept(remoteAddr string) (allow bool, rejectBanner string)
	OnTransportReadError(remoteAddr string, err error)
	OnFrameDecodeError(remoteAddr string, invalidLen int, err error)
}

// DefaultConfig 返回传输层合理默认值（心跳、队列、重连、拨号、MTU）。
//
// 返回：可直接用于 Dial/ListenTLS；损耗型 underlay（如 ZeroTier）宜调大 HeartbeatTimeout、缩短 DialTimeout/ReconnectMax。
// 副作用：无；纯函数。
func DefaultConfig() Config {
	return Config{
		HeartbeatInterval: timeutil.Seconds(netutil.DefaultHeartbeatIntervalSec),
		HeartbeatTimeout:  timeutil.Seconds(netutil.DefaultHeartbeatTimeoutSec),
		WriteTimeout:      15 * time.Second,
		MaxQueueSize:      netutil.DefaultSendQueueSize,
		ReconnectInitial:  timeutil.Seconds(netutil.DefaultReconnectInitialSec),
		ReconnectMax:      timeutil.Seconds(netutil.DefaultReconnectMaxSec),
		DialTimeout:       timeutil.Seconds(netutil.DefaultDialTimeoutSec),
		MTU:               netutil.DefaultMTU,
	}
}

// EffectiveDialTimeout 拨号超时（默认 netutil.DefaultDialTimeoutSec）。
func (c Config) EffectiveDialTimeout() time.Duration {
	if c.DialTimeout > 0 {
		return c.DialTimeout
	}
	return timeutil.Seconds(netutil.DefaultDialTimeoutSec)
}

// EffectiveReconnectMax 重连退避上限（默认 netutil.DefaultReconnectMaxSec）。
func (c Config) EffectiveReconnectMax() time.Duration {
	if c.ReconnectMax > 0 {
		return c.ReconnectMax
	}
	return timeutil.Seconds(netutil.DefaultReconnectMaxSec)
}

// EffectiveReconnectInitial 首次退避（默认 netutil.DefaultReconnectInitialSec）。
func (c Config) EffectiveReconnectInitial() time.Duration {
	if c.ReconnectInitial > 0 {
		return c.ReconnectInitial
	}
	return timeutil.Seconds(netutil.DefaultReconnectInitialSec)
}

// AfterDisconnectPause 曾 Connected 后断开再拨前的短暂停顿（立即重试，避免 tight loop）。
func AfterDisconnectPause() time.Duration {
	return 200 * time.Millisecond
}
