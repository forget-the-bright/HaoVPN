package transport

import "errors"

// ProbeMTU 发送 MTU 探测帧（FrameTypeMTUProbe），用于路径 MTU 发现。
//
// 参数：size — 探测载荷字节数；队列满时返回 error（WARN 经 noteSendQueueFull 限频）。
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
		c.noteSendQueueFull(FrameTypeMTUProbe)
		return errors.New("send queue full")
	}
}
