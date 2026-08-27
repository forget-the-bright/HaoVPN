package sessionmgr

// PacketConn 会话管理器所需的传输连接能力（窄接口，便于 mock 与解耦 transport 包）。
//
// 实现方：*transport.Conn（生产环境）。
// 契约：Send 发送数据帧；Close 幂等；RemoteAddr 返回 host:port。
type PacketConn interface {
	Send(payload []byte) error
	Close() error
	RemoteAddr() string
}
