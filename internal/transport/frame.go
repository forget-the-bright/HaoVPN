package transport

import (
	"encoding/binary"
	"fmt"
)

const (
	// FrameHeaderSize 帧头为 4 字节大端长度前缀。
	FrameHeaderSize = 4
	// MaxFrameSize 单帧 payload 上限（含 1 字节类型）。
	MaxFrameSize = 1 << 20 // 1 MiB
	// FrameTypeData 数据帧（隧道内层 IP 包密文）。
	FrameTypeData byte = 0x01
	// FrameTypeHeartbeat 心跳保活帧。
	FrameTypeHeartbeat byte = 0x02
	// FrameTypeMTUProbe 路径 MTU 探测帧。
	FrameTypeMTUProbe byte = 0x03
	// FrameTypeHandshake 隧道身份握手（JSON payload）。
	FrameTypeHandshake byte = 0x04
)

// State 表示 TLS 传输连接的生命周期状态（Conn 内部 atomic 存储）。
//
// 取值：
//   StateDisconnected — 未连接或拨号失败后的空闲态。
//   StateConnecting — Dial 进行中，尚未完成 TLS 握手。
//   StateConnected — 握手完成，可收发数据帧与心跳。
//   StateDisconnecting — Close 已触发，正在关闭底层连接。
//   StateClosed — 连接已关闭，不可再 Send。
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

// Frame 解码后的传输层一帧（类型字节 + 载荷）。
//
// 字段：
//   Type — 帧类型常量（FrameTypeData/Heartbeat/MTUProbe/Handshake）。
//   Payload — 类型字节之后的载荷副本；心跳帧通常为空。
type Frame struct {
	Type    byte
	Payload []byte
}

// EncodeFrame 编码为 [4 字节大端长度][1 字节类型][payload] 线格式。
//
// 参数：typ — 帧类型；payload — 可为 nil 或空。
// 返回：完整线格式字节；err 为 body 超过 MaxFrameSize。
// 副作用：分配新切片；无网络 I/O。
// 并发：可多 goroutine 并行编码。
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

// Decoder 从字节流重组完整帧，处理 TCP/TLS 粘包与半包。
//
// 字段：
//   buf — 未消费的字节累积区；Feed 追加、完整帧取出后截断。
//
// 线程安全：非并发安全；每条 Conn 独占一个 Decoder，仅 readLoop 调用。
type Decoder struct {
	buf []byte
}

// NewDecoder 创建空的帧解码器，供 Conn.readLoop 使用。
//
// 返回：*Decoder 初始 buf 为空。
// 副作用：无。
// 并发：每条 Conn 创建一个，readLoop 独占。
func NewDecoder() *Decoder { return &Decoder{} }

// Feed 追加读到的字节并返回当前已完整的帧列表。
//
// 参数：data — TLS Read 本次读到的字节（可为半帧）。
// 返回：本次 Feed 后新完成的 []Frame（可能为空）；err 为帧长度非法（≤0 或 >MaxFrameSize）。
// 副作用：修改内部 buf；完整帧的 Payload 为独立副本。
// 并发：仅 readLoop 调用。
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
