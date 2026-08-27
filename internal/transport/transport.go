// Package transport implements TLS-TCP framed transport with heartbeat and reconnect.
package transport

import (
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"haovpn/internal/logger"
)

const (
	// FrameHeaderSize is the 4-byte big-endian length prefix.
	FrameHeaderSize = 4
	// MaxFrameSize limits a single frame payload.
	MaxFrameSize = 1 << 20 // 1 MiB
	// FrameTypeData is a data frame.
	FrameTypeData byte = 0x01
	// FrameTypeHeartbeat is a keepalive frame.
	FrameTypeHeartbeat byte = 0x02
	// FrameTypeMTUProbe is used for path MTU discovery.
	FrameTypeMTUProbe byte = 0x03
	// FrameTypeHandshake 隧道身份握手（JSON payload）。
	FrameTypeHandshake byte = 0x04
)

// State represents connection lifecycle.
type State int32

const (
	StateDisconnected State = iota
	StateConnecting
	StateConnected
	StateDisconnecting
	StateClosed
)

func (s State) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateDisconnecting:
		return "disconnecting"
	case StateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// Config holds transport parameters.
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

// DefaultConfig returns sensible defaults.
// 损耗型 underlay（如 ZeroTier）：HeartbeatTimeout 宜长；DialTimeout/ReconnectMax 宜短以便快速重试。
func DefaultConfig() Config {
	return Config{
		HeartbeatInterval: 15 * time.Second,
		HeartbeatTimeout:  90 * time.Second,
		WriteTimeout:      15 * time.Second,
		MaxQueueSize:      256,
		ReconnectInitial:  1 * time.Second,
		ReconnectMax:      3 * time.Second,
		DialTimeout:       3 * time.Second,
		MTU:               1420,
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

// Frame is a decoded transport frame.
type Frame struct {
	Type    byte
	Payload []byte
}

// EncodeFrame builds [4-byte len][type+payload].
func EncodeFrame(typ byte, payload []byte) ([]byte, error) {
	body := make([]byte, 1+len(payload))
	body[0] = typ
	copy(body[1:], payload)
	if len(body) > MaxFrameSize {
		return nil, fmt.Errorf("frame too large: %d", len(body))
	}
	out := make([]byte, FrameHeaderSize+len(body))
	binary.BigEndian.PutUint32(out[:FrameHeaderSize], uint32(len(body)))
	copy(out[FrameHeaderSize:], body)
	return out, nil
}

// Decoder reassembles frames from a byte stream (handles sticky packets).
type Decoder struct {
	buf []byte
}

// NewDecoder creates a frame decoder.
func NewDecoder() *Decoder { return &Decoder{} }

// Feed appends data and returns complete frames.
func (d *Decoder) Feed(data []byte) ([]Frame, error) {
	d.buf = append(d.buf, data...)
	var frames []Frame
	for {
		if len(d.buf) < FrameHeaderSize {
			break
		}
		n := int(binary.BigEndian.Uint32(d.buf[:FrameHeaderSize]))
		if n <= 0 || n > MaxFrameSize {
			return nil, fmt.Errorf("invalid frame length: %d", n)
		}
		total := FrameHeaderSize + n
		if len(d.buf) < total {
			break
		}
		body := d.buf[FrameHeaderSize:total]
		d.buf = d.buf[total:]
		f := Frame{Type: body[0], Payload: nil}
		if len(body) > 1 {
			f.Payload = append([]byte(nil), body[1:]...)
		}
		frames = append(frames, f)
	}
	return frames, nil
}

// Conn wraps a TLS connection with framing, heartbeat, and queue.
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

// Dial connects to addr with TLS and starts reader/writer loops.
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

// AcceptConn wraps an accepted TLS connection.
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

// State returns current connection state.
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

// Close initiates graceful close.
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
		_ = c.tls.SetReadDeadline(time.Now().Add(c.cfg.HeartbeatTimeout))
		n, err := c.tls.Read(buf)
		if err != nil {
			if !errors.Is(err, io.EOF) && c.State() == StateConnected {
				logger.Warn("transport read error: %v", err)
			}
			return
		}
		frames, err := c.decoder.Feed(buf[:n])
		if err != nil {
			logger.Error("frame decode error: %v", err)
			return
		}
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
			frame, _ := EncodeFrame(FrameTypeHeartbeat, nil)
			select {
			case c.sendQ <- frame:
			default:
			}
			last := time.Unix(0, c.lastHB.Load())
			if time.Since(last) > c.cfg.HeartbeatTimeout {
				logger.Warn("heartbeat timeout, closing connection")
				c.Close()
				return
			}
		}
	}
}

// ReconnectClient manages automatic reconnection with exponential backoff.
type ReconnectClient struct {
	cfg       Config
	addr      string
	tlsCfg    *tls.Config
	onData    func([]byte)
	onConnect func(*Conn)
	mu        sync.Mutex
	conn      *Conn
	stop      chan struct{}
	once      sync.Once
}

// NewReconnectClient creates a client that auto-reconnects.
func NewReconnectClient(addr string, tlsCfg *tls.Config, cfg Config, onData func([]byte), onConnect func(*Conn)) *ReconnectClient {
	return &ReconnectClient{
		cfg:       cfg,
		addr:      addr,
		tlsCfg:    tlsCfg,
		onData:    onData,
		onConnect: onConnect,
		stop:      make(chan struct{}),
	}
}

// Start begins the reconnect loop.
func (r *ReconnectClient) Start() {
	go r.loop()
}

func (r *ReconnectClient) loop() {
	backoff := r.cfg.EffectiveReconnectInitial()
	maxBackoff := r.cfg.EffectiveReconnectMax()
	dialTO := r.cfg.EffectiveDialTimeout()
	for {
		select {
		case <-r.stop:
			return
		default:
		}
		conn, err := Dial(r.addr, r.tlsCfg, r.cfg, r.onData, func(err error) {
			logger.Info("transport disconnected from %s", r.addr)
		})
		if err != nil {
			logger.Warn("reconnect failed: %v, retry in %s dial_timeout=%s backoff=%s", err, backoff, dialTO, backoff)
			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		backoff = r.cfg.EffectiveReconnectInitial()
		r.mu.Lock()
		r.conn = conn
		r.mu.Unlock()
		if r.onConnect != nil {
			r.onConnect(conn)
		}
		// Wait until connection closes
		for conn.State() != StateClosed && conn.State() != StateDisconnected {
			time.Sleep(100 * time.Millisecond)
			select {
			case <-r.stop:
				conn.Close()
				return
			default:
			}
		}
		// 曾 Connected 后断开：立即重拨（短暂停顿），避免再等一整轮退避。
		pause := AfterDisconnectPause()
		logger.Info("will reconnect to %s in %s (after disconnect) dial_timeout=%s", r.addr, pause, dialTO)
		time.Sleep(pause)
		backoff = r.cfg.EffectiveReconnectInitial()
	}
}

// Stop stops reconnection.
func (r *ReconnectClient) Stop() {
	r.once.Do(func() { close(r.stop) })
	r.mu.Lock()
	if r.conn != nil {
		r.conn.Close()
	}
	r.mu.Unlock()
}

// Conn returns the active connection (may be nil).
func (r *ReconnectClient) Conn() *Conn {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.conn
}

// Server listens for TLS connections.
type Server struct {
	cfg      Config
	listener net.Listener
	onConn   func(*Conn)
}

// ListenTLS starts a TLS listener.
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

// Close stops the server listener.
func (s *Server) Close() error {
	return s.listener.Close()
}

// Addr 返回监听器地址（用于测试获取动态端口）。
func (s *Server) Addr() net.Addr {
	return s.listener.Addr()
}

// ProbeMTU sends an MTU probe frame.
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
