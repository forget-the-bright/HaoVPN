package tunnel

import (
	"errors"

	"haovpn/internal/autherr"
	"haovpn/internal/logger"
	"haovpn/internal/transport"
)

// rejectHandshake 向客户端发送 handshake_err 并关闭连接。
//
// 参数：err — 失败原因；写入可读 error 文案 + 稳定 code（autherr.HandshakeCode）。
// 副作用：打 Warn 日志；可选 OnHandshakeReject 记探针；SendRawSync 握手失败帧；conn.Close。
// 注意：探针分类在 ProbeRecorder 实现内完成，本文件不 import probedefense。
func (h *ServerHandler) rejectHandshake(conn *transport.Conn, err error) {
	if err == nil {
		err = errors.New("握手失败")
	}
	msg := err.Error()
	code := autherr.HandshakeCode(err)
	remote := conn.RemoteAddr()
	logger.Warn("握手拒绝: remote=%s code=%s %s", remote, code, msg)
	if h.Probe != nil {
		h.Probe.OnHandshakeReject(remote, err)
	}
	errBytes, encErr := EncodeHandshakeErrCode(code, msg)
	if encErr != nil {
		logger.Warn("编码握手错误帧失败: %v", encErr)
	} else if sendErr := conn.SendRawSync(transport.FrameTypeHandshake, errBytes); sendErr != nil {
		logger.Warn("发送握手错误帧失败 remote=%s: %v", remote, sendErr)
	}
	conn.Close()
}
