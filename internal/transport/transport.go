package transport

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
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
type Config struct {
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
	WriteTimeout      time.Duration
	MaxQueueSize      int
	ReconnectInitial  time.Duration
	ReconnectMax      time.Duration
	DialTimeout       time.Duration // TCP 拨号超时；0 表示用默认 3s（损耗链路避免空等 10s）
	MTU               int
}

// DefaultConfig 返回传输层合理默认值（心跳、队列、重连、拨号、MTU）。
//
// 返回：可直接用于 Dial/ListenTLS；损耗型 underlay（如 ZeroTier）宜调大 HeartbeatTimeout、缩短 DialTimeout/ReconnectMax。
// 副作用：无；纯函数。
func DefaultConfig() Config {
	return Config{
		HeartbeatInterval: time.Duration(netutil.DefaultHeartbeatIntervalSec) * time.Second,
		HeartbeatTimeout:  time.Duration(netutil.DefaultHeartbeatTimeoutSec) * time.Second,
		WriteTimeout:      15 * time.Second,
		MaxQueueSize:      256,
		ReconnectInitial:  time.Duration(netutil.DefaultReconnectInitialSec) * time.Second,
		ReconnectMax:      time.Duration(netutil.DefaultReconnectMaxSec) * time.Second,
		DialTimeout:       time.Duration(netutil.DefaultDialTimeoutSec) * time.Second,
		MTU:               netutil.DefaultMTU,
	}
}

// EffectiveDialTimeout 拨号超时（默认 3s）。
func (c Config) EffectiveDialTimeout() time.Duration {
	if c.DialTimeout > 0 {
		return c.DialTimeout
	}
	return 3 * time.Second
}

// EffectiveReconnectMax 重连退避上限（默认 3s）。
func (c Config) EffectiveReconnectMax() time.Duration {
	if c.ReconnectMax > 0 {
		return c.ReconnectMax
	}
	return 3 * time.Second
}

// EffectiveReconnectInitial 首次退避（默认 1s）。
func (c Config) EffectiveReconnectInitial() time.Duration {
	if c.ReconnectInitial > 0 {
		return c.ReconnectInitial
	}
	return time.Second
}

// AfterDisconnectPause 曾 Connected 后断开再拨前的短暂停顿（立即重试，避免 tight loop）。
func AfterDisconnectPause() time.Duration {
	return 200 * time.Millisecond
}

// Conn 封装 TLS 连接上的分帧、心跳、发送队列与状态机。
//
// 字段：
//   cfg — 心跳/超时/队列/MTU 配置副本。
//   raw — 底层 net.Conn（Dial 或 Accept 所得）。
//   tls — TLS 会话；读写均经此层。
//   decoder — 粘包拆帧解码器。
//   state — 连接生命周期（State，atomic）。
//   mu — 保护 onData/onClose 回调指针。
//   sendQ — 待发编码帧队列；满则 SendRaw 失败。
//   onData — 收到 Data/Handshake 帧时的回调。
//   onClose — Close 或读写出错时的回调。
//   lastHB — 最近一次收到对端活跃帧的时间戳（纳秒，atomic）。
//   closed — Close 时关闭，通知 read/write/heartbeat 协程退出。
//   closeOnce — 保证 Close 只执行一次。
//
// 并发：内部启动 readLoop、writeLoop、heartbeatLoop；须通过 Close 停止。
type Conn struct {
	cfg       Config
	raw       net.Conn
	tls       *tls.Conn
	decoder   Decoder
	state     atomic.Int32
	mu        sync.Mutex
	sendQ     chan []byte
	onData    func([]byte)
	onClose   func(error)
	lastHB    atomic.Int64
	closed    chan struct{}
	closeOnce sync.Once
}

// Dial 以 TLS 连接 addr 并启动读/写/心跳协程（客户端出站）。
//
// 参数：addr — host:port；tlsCfg 非 nil；onData/onClose 可为 nil。
// 返回：*Conn 已 StateConnected；err 为 TCP 或 TLS 握手失败。
// 副作用：启动 3 个 goroutine；失败时不留泄漏连接。
func Dial(addr string, tlsCfg *tls.Config, cfg Config, onData func([]byte), onClose func(error)) (*Conn, error) {
	if cfg.MaxQueueSize <= 0 {
		cfg.MaxQueueSize = 256
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
	tlsConn := tls.Client(raw, tlsCfg)
	if err := tlsConn.Handshake(); err != nil {
		raw.Close()
		c.setState(StateDisconnected)
		return nil, fmt.Errorf("tls handshake: %w", err)
	}
	c.raw = raw
	c.tls = tlsConn
	c.setState(StateConnected)
	c.touchHB()
	go c.readLoop()
	go c.writeLoop()
	go c.heartbeatLoop()
	logger.Info("transport connected to %s", addr)
	return c, nil
}

// AcceptConn 包装已 Accept 的 TLS 连接并启动读写循环（服务端入站）。
//
// 参数：tlsConn — 已完成 TLS 握手的连接；回调同 Dial。
// 返回：*Conn 已 StateConnected；不负责关闭底层 net.Conn（Close 时关）。
func AcceptConn(tlsConn *tls.Conn, cfg Config, onData func([]byte), onClose func(error)) *Conn {
	if cfg.MaxQueueSize <= 0 {
		cfg.MaxQueueSize = 256
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
	go c.readLoop()
	go c.writeLoop()
	go c.heartbeatLoop()
	return c
}

func (c *Conn) setState(s State) { c.state.Store(int32(s)) }

// State 返回当前连接生命周期状态（atomic 读取）。
func (c *Conn) State() State { return State(c.state.Load()) }

func (c *Conn) touchHB() { c.lastHB.Store(time.Now().UnixNano()) }

// RemoteAddr 返回底层 TCP 远端地址。
func (c *Conn) RemoteAddr() string {
	if c.raw == nil {
		return ""
	}
	return c.raw.RemoteAddr().String()
}

// SetOnData 动态设置数据回调（握手完成后切换为数据转发）。
func (c *Conn) SetOnData(fn func([]byte)) {
	c.mu.Lock()
	c.onData = fn
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
// Send 发送数据帧（隧道内层 IP 包密文）。
func (c *Conn) Send(payload []byte) error {
	if c.State() != StateConnected {
		return errors.New("not connected")
	}
	return c.SendRaw(FrameTypeData, payload)
}

// Close 发起优雅关闭：置 Disconnecting、关闭 closed channel、关 TLS、触发 onClose。
//
// 返回：TLS Close 的错误（仅首次调用有效）；重复调用安全。
// 副作用：read/write/heartbeat 协程退出；State 变为 Closed。
func (c *Conn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		c.setState(StateDisconnecting)
		close(c.closed)
		if c.tls != nil {
			err = c.tls.Close()
		}
		c.setState(StateClosed)
		if c.onClose != nil {
			c.onClose(nil)
		}
	})
	return err
}

func (c *Conn) readLoop() {
	defer c.Close()
	buf := GetBuffer(c.cfg.MTU + FrameHeaderSize + 64)
	defer PutBuffer(buf)
	for {
		select {
		case <-c.closed:
			return
		default:
		}
		// --- 阶段 1：带读超时的 TLS Read ---
		_ = c.tls.SetReadDeadline(time.Now().Add(c.cfg.HeartbeatTimeout))
		n, err := c.tls.Read(buf)
		if err != nil {
			if !errors.Is(err, io.EOF) && c.State() == StateConnected {
				logger.Warn("transport read error: %v", err)
			}
			return
		}
		// --- 阶段 2：粘包拆帧 ---
		frames, err := c.decoder.Feed(buf[:n])
		if err != nil {
			logger.Error("frame decode error: %v", err)
			return
		}
		// --- 阶段 3：按帧类型分发（心跳刷新 / 回调 onData） ---
		for _, f := range frames {
			switch f.Type {
			case FrameTypeHeartbeat, FrameTypeMTUProbe:
				c.touchHB()
			case FrameTypeData:
				c.touchHB()
				c.mu.Lock()
				fn := c.onData
				c.mu.Unlock()
				if fn != nil {
					fn(f.Payload)
				}
			case FrameTypeHandshake:
				c.touchHB()
				c.mu.Lock()
				fn := c.onData
				c.mu.Unlock()
				if fn != nil {
					fn(f.Payload)
				}
			}
		}
	}
}

func (c *Conn) writeLoop() {
	for {
		select {
		case <-c.closed:
			return
		case frame := <-c.sendQ:
			// --- 阶段 1：带写超时的 TLS Write ---
			_ = c.tls.SetWriteDeadline(time.Now().Add(c.cfg.WriteTimeout))
			if _, err := c.tls.Write(frame); err != nil {
				logger.Warn("transport write error: %v", err)
				c.Close()
				return
			}
		}
	}
}

func (c *Conn) heartbeatLoop() {
	ticker := time.NewTicker(c.cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.closed:
			return
		case <-ticker.C:
			// --- 阶段 1：入队心跳帧 ---
			frame, _ := EncodeFrame(FrameTypeHeartbeat, nil)
			select {
			case c.sendQ <- frame:
			default:
			}
			// --- 阶段 2：检查对端静默超时 ---
			last := time.Unix(0, c.lastHB.Load())
			if time.Since(last) > c.cfg.HeartbeatTimeout {
				logger.Warn("heartbeat timeout, closing connection")
				c.Close()
				return
			}
		}
	}
}

// Server TLS 入站监听器：acceptLoop 接受连接并交给 onConn。
//
// 字段：
//   cfg — 传给 AcceptConn 的心跳/队列配置。
//   listener — tls.Listen 所得。
//   onConn — 每建立一条 TLS 连接时的回调（tunnel 握手入口）。
type Server struct {
	cfg      Config
	listener net.Listener
	onConn   func(*Conn)
}

// ListenTLS 在 addr 上启动 TLS 监听并接受客户端连接。
//
// 参数：addr — host:port；tlsCfg — 服务端证书；cfg — 心跳/队列参数；onConn — 每接受一条 TLS 连接后回调（通常进入握手）。
// 返回：*Server 已在后台 acceptLoop；err 常见为地址占用或证书无效。
// 副作用：启动 goroutine 接受连接；日志记录 listening 地址。
// 并发：acceptLoop 与调用方并行；Stop 时须 Server.Close。
func ListenTLS(addr string, tlsCfg *tls.Config, cfg Config, onConn func(*Conn)) (*Server, error) {
	ln, err := tls.Listen("tcp", addr, tlsCfg)
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, listener: ln, onConn: onConn}
	go s.acceptLoop()
	logger.Info("transport listening on %s", addr)
	return s, nil
}

func (s *Server) acceptLoop() {
	for {
		raw, err := s.listener.Accept()
		if err != nil {
			logger.Info("transport listener closed: %v", err)
			return
		}
		tlsConn, ok := raw.(*tls.Conn)
		if !ok {
			raw.Close()
			continue
		}
		conn := AcceptConn(tlsConn, s.cfg, nil, func(err error) {
			logger.Debug("peer disconnected: %v", err)
		})
		if s.onConn != nil {
			s.onConn(conn)
		}
	}
}

// Close 关闭 TLS 监听器；acceptLoop 将因 Accept 错误退出。
func (s *Server) Close() error {
	return s.listener.Close()
}

// Addr 返回监听器地址（用于测试获取动态端口）。
func (s *Server) Addr() net.Addr {
	return s.listener.Addr()
}

// ProbeMTU 发送 MTU 探测帧（FrameTypeMTUProbe），用于路径 MTU 发现。
//
// 参数：size — 探测载荷字节数；队列满时返回 error 并打 Warn 日志。
func (c *Conn) ProbeMTU(size int) error {
	payload := make([]byte, size)
	frame, err := EncodeFrame(FrameTypeMTUProbe, payload)
	if err != nil {
		return err
	}
	select {
	case c.sendQ <- frame:
		return nil
	default:
		logger.Warn("transport MTU probe dropped: send queue full (max=%d)", c.cfg.MaxQueueSize)
		return errors.New("send queue full")
	}
}
