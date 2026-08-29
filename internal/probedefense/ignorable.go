package probedefense

import (
	"errors"
	"net"
	"os"
	"strings"
)

// IsIgnorableTransportError 读超时、deadline、已关闭连接等不算探针（心跳读 deadline 常态）。
//
// 返回 true 时 Guard 不应记安全事件或自动封禁。
func IsIgnorableTransportError(err error) bool {
	if err == nil {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	if errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, net.ErrClosed) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "use of closed")
}
