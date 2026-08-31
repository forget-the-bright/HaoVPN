package transport

import (
	"crypto/tls"
	"sync"
	"sync/atomic"
	"time"

	"haovpn/internal/dialerr"
	"haovpn/internal/logger"
	"haovpn/internal/safeutil"
)

// ReconnectClient 客户端自动重连管理器：指数退避 Dial，连接成功后回调 onConnect。
//
// 字段：
//   cfg — 心跳/重连/拨号超时配置；loop 中读取 Effective* 方法。
//   addr — 目标 host:port；不变。
//   tlsCfg — 客户端 TLS 配置；Dial 时传入。
//   onData — 收到数据帧时转发给上层（通常为 tunnel 解密入口）。
//   onConnect — 每次 Dial 成功、StateConnected 后调用；用于重新握手。
//   onDialError — 拨号/TLS 失败时可选回调（供 GUI 首次失败提示）；可为 nil。
//   mu — 保护 conn 指针读写。
//   conn — 当前活跃 *Conn；重连间隙或 Stop 后为 nil。
//   stop — Stop 时 close，通知 loop 退出。
//   once — 保证 stop channel 只关闭一次。
//   started — 防双 Start（CAS）；重复 Start 打 Warn 并忽略。
//
// 线程安全：Start/Stop/Conn 可从任意 goroutine 调用；loop 经 safeutil.GoSafe 运行。
type ReconnectClient struct {
	cfg         Config
	addr        string
	tlsCfg      *tls.Config
	onData      func([]byte)
	onConnect   func(*Conn)
	onDialError func(error)
	mu          sync.Mutex
	conn        *Conn
	stop        chan struct{}
	once        sync.Once
	started     atomic.Bool
}

// NewReconnectClient 创建带自动重连的客户端传输管理器。
//
// 参数：addr — 服务端地址；tlsCfg — 非 nil；cfg — 退避/拨号参数；onData/onConnect 可为 nil。
// 返回：*ReconnectClient 须调用 Start 启动 loop；未 Start 前 Conn 恒为 nil。
// 副作用：无；不发起网络连接。
// 并发：返回后单实例仅应 Start 一次（重复 Start 被忽略）。
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

// SetOnDialError 设置拨号失败回调（须在 Start 前调用）。
func (r *ReconnectClient) SetOnDialError(fn func(error)) {
	r.onDialError = fn
}

// Start 在后台 goroutine 启动重连循环（GoSafe；防双 Start）。
//
// 副作用：启动 loop goroutine，持续 Dial 直至 Stop。
// 并发：重复调用打 Warn 并忽略，不会启动第二个 loop。
func (r *ReconnectClient) Start() {
	if !r.started.CompareAndSwap(false, true) {
		logger.Warn("ReconnectClient.Start 重复调用已忽略 addr=%s", r.addr)
		return
	}
	safeutil.GoSafe("transport-reconnect", r.loop)
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
		// --- 阶段 1：拨号与 TLS 握手 ---
		conn, err := Dial(r.addr, r.tlsCfg, r.cfg, r.onData, func(err error) {
			logger.Info("传输已断开 addr=%s", r.addr)
		})
		if err != nil {
			if dialerr.IsFatalDialError(err) {
				logger.Warn("重连已停止（致命拨号错误）addr=%s: %v", r.addr, err)
				if r.onDialError != nil {
					r.onDialError(err)
				}
				return
			}
			logger.Warn("重连失败，将退避重试 addr=%s err=%v dial_timeout=%s backoff=%s", r.addr, err, dialTO, backoff)
			if r.onDialError != nil {
				r.onDialError(err)
			}
			time.Sleep(backoff)
			backoff = safeutil.ExpBackoff(backoff, maxBackoff)
			continue
		}
		backoff = r.cfg.EffectiveReconnectInitial()
		r.mu.Lock()
		r.conn = conn
		r.mu.Unlock()
		// --- 阶段 2：通知上层重新握手 ---
		if r.onConnect != nil {
			r.onConnect(conn)
		}
		// --- 阶段 3：等待连接断开（Conn.Done，禁止 100ms 空转）---
		select {
		case <-r.stop:
			conn.Close()
			return
		case <-conn.Done():
		}
		r.mu.Lock()
		if r.conn == conn {
			r.conn = nil
		}
		r.mu.Unlock()
		// --- 阶段 4：断线后短暂停顿再重拨 ---
		pause := AfterDisconnectPause()
		logger.Info("将重连 addr=%s pause=%s dial_timeout=%s", r.addr, pause, dialTO)
		time.Sleep(pause)
		backoff = r.cfg.EffectiveReconnectInitial()
	}
}

// Stop 停止重连循环并关闭当前 Conn。
//
// 副作用：close(stop) 一次；关闭 r.conn（若存在）；loop 退出后不再 Dial。
// 并发：可安全多次调用（once 保护）；与 Start 后的 loop 并发安全。
func (r *ReconnectClient) Stop() {
	r.once.Do(func() { close(r.stop) })
	r.mu.Lock()
	if r.conn != nil {
		r.conn.Close()
	}
	r.mu.Unlock()
}

// Conn 返回当前活跃连接；重连间隙或 Stop 后可能为 nil。
//
// 返回：*Conn 快照；调用方不应长期持有指针（重连后可能失效）。
// 副作用：无；持 mu 短暂加锁。
// 并发：可与 loop 并发调用。
func (r *ReconnectClient) Conn() *Conn {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.conn
}
