package transport

import (
	"crypto/tls"
	"net"

	"haovpn/internal/logger"
)

// Server TLS 入站监听器：acceptLoop 接受连接并交给 onConn。
//
// 字段：
//   cfg — 传给 AcceptConn 的心跳/队列配置。
//   listener — tls.Listen 所得。
//   onConn — 每建立一条 TLS 连接时的回调（tunnel 握手入口）。
type Server struct {
	cfg      Config
	listener net.Listener
	onConn   func(*Conn)
}

// ListenTLS 在 addr 上启动 TLS 监听并接受客户端连接。
//
// 参数：addr — host:port；tlsCfg — 服务端证书；cfg — 心跳/队列参数；onConn — 每接受一条 TLS 连接后回调（通常进入握手）。
// 返回：*Server 已在后台 acceptLoop；err 常见为地址占用或证书无效。
// 副作用：启动 goroutine 接受连接；日志记录 listening 地址。
// 并发：acceptLoop 与调用方并行；Stop 时须 Server.Close。
func ListenTLS(addr string, tlsCfg *tls.Config, cfg Config, onConn func(*Conn)) (*Server, error) {
	ln, err := tls.Listen("tcp", addr, tlsCfg)
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, listener: ln, onConn: onConn}
	go s.acceptLoop()
	logger.Info("transport listening on %s", addr)
	return s, nil
}

func (s *Server) acceptLoop() {
	for {
		raw, err := s.listener.Accept()
		if err != nil {
			logger.Info("transport listener closed: %v", err)
			return
		}
		remote := raw.RemoteAddr().String()
		if s.cfg.Probe != nil && !s.cfg.Probe.AllowAccept(remote) {
			_ = raw.Close()
			continue
		}
		tlsConn, ok := raw.(*tls.Conn)
		if !ok {
			raw.Close()
			continue
		}
		conn := AcceptConn(tlsConn, s.cfg, nil, func(err error) {
			logger.Debug("peer disconnected: %v", err)
		})
		if s.onConn != nil {
			s.onConn(conn)
		}
	}
}

// Close 关闭 TLS 监听器；acceptLoop 将因 Accept 错误退出。
func (s *Server) Close() error {
	return s.listener.Close()
}

// Addr 返回监听器地址（用于测试获取动态端口）。
func (s *Server) Addr() net.Addr {
	return s.listener.Addr()
}
