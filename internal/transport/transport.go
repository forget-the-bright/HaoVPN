package transport

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"haovpn/internal/dialerr"
	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/safeutil"
)

// Conn 封装 TLS 连接上的分帧、心跳、发送队列与状态机。
//
// 字段：
//   cfg — 心跳/超时/队列/MTU 配置副本。
//   raw — 底层 net.Conn（Dial 或 Accept 所得）。
//   tls — TLS 会话；读写均经此层。
//   decoder — 粘包拆帧解码器。
//   state — 连接生命周期（State，atomic）。
//   mu — 保护 onData/onHandshake/onClose 回调指针。
//   sendQ — 待发编码帧队列；满则 SendRaw 失败。
//   onData — 收到 Data 帧时的回调（鉴权后隧道密文）。
//   onHandshake — 收到 Handshake 帧时的回调；nil 时 Handshake 回退到 onData（兼容旧测）。
//   onClose — Close 或读写出错时的回调。
//   lastHB — 最近一次收到对端活跃帧的时间戳（纳秒，atomic）。
//   hbPause — applyPolicy 配网期间为 true：仍发心跳，但不因静默超时 Close。
//   closed — Close 时关闭，通知 read/write/heartbeat 协程退出。
//   closeOnce — 保证 Close 只执行一次。
//
// 并发：内部启动 readLoop、writeLoop、heartbeatLoop；须通过 Close 停止。
type Conn struct {
	cfg         Config
	raw         net.Conn
	tls         *tls.Conn
	decoder     Decoder
	state       atomic.Int32
	mu          sync.Mutex
	writeMu     sync.Mutex // 保护 tls.Write（writeLoop 与 SendRawSync）
	sendQ       chan []byte
	onData      func([]byte)
	onHandshake func([]byte)
	onClose     func(error)
	lastHB      atomic.Int64
	hbPause     atomic.Bool // applyPolicy 期间为 true：仍发心跳，但不因静默超时 Close
	closed      chan struct{}
	closeOnce   sync.Once
}

// Dial 以 TLS 连接 addr 并启动读/写/心跳协程（客户端出站）。
//
// 参数：addr — host:port；tlsCfg 非 nil；onData/onClose 可为 nil。
// 返回：*Conn 已 StateConnected；err 为 TCP 或 TLS 握手失败。
// 副作用：启动 3 个 goroutine；失败时不留泄漏连接。
func Dial(addr string, tlsCfg *tls.Config, cfg Config, onData func([]byte), onClose func(error)) (*Conn, error) {
	if cfg.MaxQueueSize <= 0 {
		cfg.MaxQueueSize = netutil.DefaultSendQueueSize
	}
	c := &Conn{
		cfg:     cfg,
		onData:  onData,
		onClose: onClose,
		sendQ:   make(chan []byte, cfg.MaxQueueSize),
		closed:  make(chan struct{}),
	}
	c.setState(StateConnecting)
	raw, err := net.DialTimeout("tcp", addr, cfg.EffectiveDialTimeout())
	if err != nil {
		c.setState(StateDisconnected)
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	raw, err = readProbeRejectBanner(raw)
	if err != nil {
		c.setState(StateDisconnected)
		if errors.Is(err, dialerr.ErrIPBanned) {
			return nil, err
		}
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	tlsConn := tls.Client(raw, tlsCfg)
	if err := tlsConn.Handshake(); err != nil {
		raw.Close()
		c.setState(StateDisconnected)
		if classified := dialerr.ClassifyTLSHandshakeErr(err); errors.Is(classified, dialerr.ErrPlaintextBeforeTLS) {
			// 不直接断言 ErrIPBanned：也可能是连错端口；由 FormatDialError 给出双因提示。
			logger.Warn("tls 握手读到明文（可能封禁 banner 晚到或非隧道口） addr=%s: %v", addr, err)
			return nil, classified
		}
		return nil, fmt.Errorf("tls handshake: %w", err)
	}
	c.raw = raw
	c.tls = tlsConn
	c.setState(StateConnected)
	c.touchHB()
	// GoSafe：读写/心跳 panic 不得拖垮整个客户端进程。
	safeutil.GoSafe("transport-read", c.readLoop)
	safeutil.GoSafe("transport-write", c.writeLoop)
	safeutil.GoSafe("transport-heartbeat", c.heartbeatLoop)
	logger.Info("transport connected to %s", addr)
	return c, nil
}

// AcceptConn 包装已 Accept 的 TLS 连接并启动读写循环（服务端入站）。
//
// 参数：tlsConn — 已完成 TLS 握手的连接；回调同 Dial。
// 返回：*Conn 已 StateConnected；不负责关闭底层 net.Conn（Close 时关）。
func AcceptConn(tlsConn *tls.Conn, cfg Config, onData func([]byte), onClose func(error)) *Conn {
	if cfg.MaxQueueSize <= 0 {
		cfg.MaxQueueSize = netutil.DefaultSendQueueSize
	}
	c := &Conn{
		cfg:     cfg,
		raw:     tlsConn.NetConn(),
		tls:     tlsConn,
		onData:  onData,
		onClose: onClose,
		sendQ:   make(chan []byte, cfg.MaxQueueSize),
		closed:  make(chan struct{}),
	}
	c.setState(StateConnected)
	c.touchHB()
	// GoSafe：读写/心跳 panic 不得拖垮整个服务端进程。
	safeutil.GoSafe("transport-read", c.readLoop)
	safeutil.GoSafe("transport-write", c.writeLoop)
	safeutil.GoSafe("transport-heartbeat", c.heartbeatLoop)
	return c
}

func (c *Conn) setState(s State) { c.state.Store(int32(s)) }

// State 返回当前连接生命周期状态（atomic 读取）。
func (c *Conn) State() State { return State(c.state.Load()) }

// LastPeerActivity 对端最近活跃时间（心跳/数据/握手帧），供 sessionmgr 判定半死会话。
func (c *Conn) LastPeerActivity() time.Time {
	ns := c.lastHB.Load()
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

func (c *Conn) touchHB() { c.lastHB.Store(time.Now().UnixNano()) }

// SetHeartbeatTimeoutPaused 暂停/恢复「对端静默超时关连接」。
//
// 用途：客户端 applyPolicy 配 TUN/路由可能数十秒到两分钟，期间若对端心跳偶发缺失，
// 不应掐掉刚鉴权成功的隧道（否则 session_abandoned）。仍照常发送本端心跳。
func (c *Conn) SetHeartbeatTimeoutPaused(paused bool) {
	if c == nil {
		return
	}
	c.hbPause.Store(paused)
	if !paused {
		c.touchHB() // 恢复时刷新，避免立刻误判超时
	}
}

// RemoteAddr 返回底层 TCP 远端地址。
func (c *Conn) RemoteAddr() string {
	if c.raw == nil {
		return ""
	}
	return c.raw.RemoteAddr().String()
}

// SetOnData 动态设置 Data 帧回调（握手完成后切换为解密转发）。
func (c *Conn) SetOnData(fn func([]byte)) {
	c.mu.Lock()
	c.onData = fn
	c.mu.Unlock()
}

// SetOnHandshake 设置 Handshake 帧回调（鉴权等待期间专用，勿与 Data 混用）。
//
// 为何分离：Data 常为密文以 \\x00 开头，若与 Handshake 共用 onData，
// json.Unmarshal 会报 invalid character '\\x00'（手动重连竞态时尤甚）。
func (c *Conn) SetOnHandshake(fn func([]byte)) {
	c.mu.Lock()
	c.onHandshake = fn
	c.mu.Unlock()
}

// SetOnClose 设置连接关闭回调（用于清理会话）。
func (c *Conn) SetOnClose(fn func(error)) {
	c.mu.Lock()
	c.onClose = fn
	c.mu.Unlock()
}

// SendRaw 发送指定类型的帧（握手等控制帧）。
func (c *Conn) SendRaw(frameType byte, payload []byte) error {
	frame, err := EncodeFrame(frameType, payload)
	if err != nil {
		return err
	}
	select {
	case c.sendQ <- frame:
		return nil
	default:
		logger.Warn("transport send queue full (max=%d), frame type=%d dropped", c.cfg.MaxQueueSize, frameType)
		return errors.New("send queue full")
	}
}

// SendRawSync 同步写出一帧（握手拒绝等须在 Close 前务必送达的场景）。
//
// 与 writeLoop 共用 writeMu，避免与队列写出并发写同一 tls.Conn。
func (c *Conn) SendRawSync(frameType byte, payload []byte) error {
	frame, err := EncodeFrame(frameType, payload)
	if err != nil {
		return err
	}
	if c.tls == nil {
		return errors.New("tls not ready")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.tls.SetWriteDeadline(time.Now().Add(c.cfg.WriteTimeout))
	_, err = c.tls.Write(frame)
	return err
}

// Send 发送数据帧（隧道内层 IP 包密文）。
func (c *Conn) Send(payload []byte) error {
	if c.State() != StateConnected {
		return errors.New("not connected")
	}
	return c.SendRaw(FrameTypeData, payload)
}

// Done 在 Close 时关闭；用于等待连接结束（替代轮询 State）。
//
// 返回：只读 channel；Close 后可读到零值。未 Close 前阻塞。
func (c *Conn) Done() <-chan struct{} {
	return c.closed
}

// Close 发起优雅关闭：置 Disconnecting、关闭 closed channel、关 TLS、触发 onClose。
//
// 返回：TLS Close 的错误（仅首次调用有效）；重复调用安全。
// 副作用：read/write/heartbeat 协程退出；State 变为 Closed。
// onClose 在 mu 下拷贝再锁外调用，避免与 SetOnClose 数据竞争。
func (c *Conn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		c.setState(StateDisconnecting)
		close(c.closed)
		if c.tls != nil {
			err = c.tls.Close()
		}
		c.setState(StateClosed)
		c.mu.Lock()
		fn := c.onClose
		c.mu.Unlock()
		if fn != nil {
			fn(nil)
		}
	})
	return err
}
