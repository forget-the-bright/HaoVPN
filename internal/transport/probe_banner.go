package transport

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"haovpn/internal/dialerr"
	"haovpn/internal/logger"
)

// bannerReadTimeout 客户端等待 TLS 前拒绝码的总时长。
//
// 成功路径上服务端在 ClientHello 前不发任何字节，故本超时会叠加到每次 Dial。
// 必须短：封禁路径依赖服务端先 Write banner 再记库；250ms 足以覆盖同城 RTT + SQLite 读。
const bannerReadTimeout = 250 * time.Millisecond

// bannerPeekSlice 单次 Peek 等待上限。
const bannerPeekSlice = 50 * time.Millisecond

// bufferedConn 将 bufio 已预读字节透传给后续 TLS 握手，避免丢失首包。
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

// readProbeRejectBanner TCP 连接后短暂探测 TLS 前明文拒绝码。
//
// 无 banner 时返回带缓冲的 conn 供 TLS 使用；不得长时间阻塞（见 bannerReadTimeout）。
// 哨兵与 banner 常量定义在 dialerr（叶子），本函数只做 I/O。
func readProbeRejectBanner(conn net.Conn) (net.Conn, error) {
	br := bufio.NewReader(conn)
	deadline := time.Now().Add(bannerReadTimeout)
	for time.Now().Before(deadline) {
		wait := time.Until(deadline)
		if wait > bannerPeekSlice {
			wait = bannerPeekSlice
		}
		if wait <= 0 {
			break
		}
		_ = conn.SetReadDeadline(time.Now().Add(wait))
		b, err := br.Peek(1)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			_ = conn.SetReadDeadline(time.Time{})
			if errors.Is(err, io.EOF) {
				_ = conn.Close()
				// 无字节的 EOF：可能是闪断或未写 banner 的拒绝，不能当成「已封禁」。
				logger.Info("tls 前连接关闭（无拒绝码） remote=%v", conn.RemoteAddr())
				return nil, dialerr.ErrClosedBeforeTLS
			}
			return &bufferedConn{Conn: conn, r: br}, nil
		}
		if len(b) == 0 || b[0] != 'H' {
			_ = conn.SetReadDeadline(time.Time{})
			return &bufferedConn{Conn: conn, r: br}, nil
		}
		line, err := br.ReadString('\n')
		_ = conn.SetReadDeadline(time.Time{})
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("read server preamble: %w", err)
		}
		banErr := dialerr.ClassifyRejectBannerLine(line)
		_ = conn.Close()
		return nil, banErr
	}
	_ = conn.SetReadDeadline(time.Time{})
	if br.Buffered() > 0 {
		peek, _ := br.Peek(br.Buffered())
		if err := dialerr.ClassifyRejectBannerBytes(peek); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	return &bufferedConn{Conn: conn, r: br}, nil
}

// WriteRejectBanner 服务端在关闭前写出拒绝码；校验写全并打日志。
//
// 参数 banner — 通常为 dialerr.BannerIPBanned / BannerSourceDenied。
func WriteRejectBanner(conn net.Conn, banner string) {
	if conn == nil || banner == "" {
		return
	}
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Write([]byte(banner))
	_ = conn.SetWriteDeadline(time.Time{})
	if err != nil {
		logger.Warn("写出 TLS 前拒绝码失败 remote=%v n=%d: %v", conn.RemoteAddr(), n, err)
		return
	}
	if n < len(banner) {
		logger.Warn("写出 TLS 前拒绝码不完整 remote=%v n=%d want=%d", conn.RemoteAddr(), n, len(banner))
	}
}
