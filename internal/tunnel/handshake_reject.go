package tunnel

import (
	"errors"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/probedefense"
	"haovpn/internal/transport"
)

// rejectHandshake 向客户端发送 handshake_err 并关闭连接。
//
// 参数：err — 失败原因；优先用 probedefense.ClassifyHandshakeReject 映射探针 signature。
// 副作用：打 Warn 日志；可选记探针事件；SendRawSync 握手失败帧；conn.Close。
func (h *ServerHandler) rejectHandshake(conn *transport.Conn, err error) {
	if err == nil {
		err = errors.New("握手失败")
	}
	msg := err.Error()
	remote := conn.RemoteAddr()
	logger.Warn("握手拒绝: remote=%s %s", remote, msg)
	if h.Probe != nil {
		ip, port := netutil.SplitRemoteAddr(remote)
		sig := probedefense.ClassifyHandshakeReject(err)
		h.Probe.RecordReject(ip, port, probedefense.PhaseHandshake, sig, msg)
	}
	errBytes, encErr := EncodeHandshakeErr(msg)
	if encErr != nil {
		logger.Warn("编码握手错误帧失败: %v", encErr)
	} else if sendErr := conn.SendRawSync(transport.FrameTypeHandshake, errBytes); sendErr != nil {
		logger.Warn("发送握手错误帧失败 remote=%s: %v", remote, sendErr)
	}
	conn.Close()
}
