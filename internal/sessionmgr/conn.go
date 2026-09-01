package sessionmgr

import "time"

// PacketConn 会话管理器所需的传输连接能力（窄接口，便于 mock 与解耦 transport 包）。
//
// 实现方：*transport.Conn（生产环境）。
// 契约：Send 发送数据帧；Close 幂等；RemoteAddr 返回 host:port。
type PacketConn interface {
	Send(payload []byte) error
	Close() error
	RemoteAddr() string
}

// PeerActivityConn 可选接口：报告对端最近活跃时间（心跳/数据帧）。
//
// *transport.Conn 实现；用于 reject_second 下识别「半死」会话并允许密码重连顶替。
type PeerActivityConn interface {
	LastPeerActivity() time.Time
}

// DrainableConn 可选接口：Close 后关闭的完成信号，供 RegisterVPN 等待旧 readLoop 退出。
//
// *transport.Conn.Done 在 Close 时关闭；不等待则旧回调仍可能在新 Crypto 挂上后入站。
type DrainableConn interface {
	Done() <-chan struct{}
}

// DataCallbackConn 可选接口：动态设置/清空 Data 帧回调。
//
// 顶替旧会话时先 SetOnData(nil)，缩小 Close 与 readLoop 退出之间的迟到包窗口。
type DataCallbackConn interface {
	SetOnData(fn func([]byte))
}
