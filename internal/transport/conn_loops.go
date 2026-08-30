package transport

import (
	"encoding/binary"
	"errors"
	"io"
	"time"

	"haovpn/internal/logger"
)

// readLoop / writeLoop / heartbeatLoop：Conn 内部读写与心跳协程。
// 与 Dial/AcceptConn 同包拆分，便于单独阅读帧分发与超时路径。

func (c *Conn) readLoop() {
	defer c.Close()
	buf := GetBuffer(c.cfg.MTU + FrameHeaderSize + 64)
	defer PutBuffer(buf)
	remote := c.RemoteAddr()
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
				// 有 Probe 时由 Guard 打带 signature 的 Warn，避免双行；超时在 Guard 内忽略
				if c.cfg.Probe != nil {
					c.cfg.Probe.OnTransportReadError(remote, err)
				} else {
					logger.Warn("transport read error: remote=%s err=%v", remote, err)
				}
			}
			return
		}
		// --- 阶段 2：粘包拆帧 ---
		frames, err := c.decoder.Feed(buf[:n])
		if err != nil {
			// 公网扫描常见：TLS 谈成后发 HTTP/AMQP 等 → 非法帧长；记 WARN 勿 ERROR+stack
			invLen := 0
			if n >= FrameHeaderSize {
				invLen = int(binary.BigEndian.Uint32(buf[:FrameHeaderSize]))
			}
			if c.cfg.Probe != nil {
				c.cfg.Probe.OnFrameDecodeError(remote, invLen, err)
			} else {
				logger.Warn("frame decode error: remote=%s err=%v", remote, err)
			}
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
			c.writeMu.Lock()
			_ = c.tls.SetWriteDeadline(time.Now().Add(c.cfg.WriteTimeout))
			_, err := c.tls.Write(frame)
			c.writeMu.Unlock()
			if err != nil {
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
