package transport

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// BannerIPBanned 服务端在 TLS 握手前写入的明文拒绝码（须以 \r\n 结尾）。
const BannerIPBanned = "HAOVPN:IP_BANNED\r\n"

// bannerReadTimeout 客户端等待服务端 TLS 前拒绝码的最长时间。
const bannerReadTimeout = 2 * time.Second

// ErrIPBanned 对端因 ip_blocks 封禁而在 TLS 前拒绝连接。
var ErrIPBanned = errors.New("ip banned by server")

// readProbeRejectBanner TCP 连接后尝试读取 TLS 前明文拒绝码。
func readProbeRejectBanner(conn net.Conn) (net.Conn, error) {
	_ = conn.SetReadDeadline(time.Now().Add(bannerReadTimeout))
	br := bufio.NewReader(conn)
	b, err := br.Peek(1)
	if err != nil {
		_ = conn.SetReadDeadline(time.Time{})
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return conn, nil
		}
		if err == io.EOF {
			_ = conn.Close()
			return nil, ErrIPBanned
		}
		return conn, nil
	}
	if len(b) == 0 || b[0] != 'H' {
		_ = conn.SetReadDeadline(time.Time{})
		return conn, nil
	}
	line, err := br.ReadString('\n')
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("read server preamble: %w", err)
	}
	if strings.HasPrefix(strings.TrimSpace(line), "HAOVPN:IP_BANNED") {
		_ = conn.Close()
		return nil, ErrIPBanned
	}
	_ = conn.Close()
	return nil, fmt.Errorf("unexpected server preamble: %s", strings.TrimSpace(line))
}
