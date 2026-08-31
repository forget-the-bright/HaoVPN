package transport

import (
	"crypto/tls"
	"io"
	"net"
	"time"

	"haovpn/internal/logger"
)

// Server TLS 入站监听器：TCP Accept → 探针检查 → TLS 握手 → 交给 onConn。
type Server struct {
	cfg      Config
	listener net.Listener
	tlsCfg   *tls.Config
	onConn   func(*Conn)
}

// ListenTLS 在 addr 上启动 TCP 监听，探针通过后完成 TLS 握手。
func ListenTLS(addr string, tlsCfg *tls.Config, cfg Config, onConn func(*Conn)) (*Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, listener: ln, tlsCfg: tlsCfg, onConn: onConn}
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
		go s.handleConn(raw)
	}
}

func (s *Server) handleConn(raw net.Conn) {
	remote := raw.RemoteAddr().String()
	if s.cfg.Probe != nil {
		allow, banner := s.cfg.Probe.CheckAccept(remote)
		if !allow {
			// 先写拒绝码再关连接；记库已在 CheckAccept 内异步，避免阻塞写出。
			WriteRejectBanner(raw, banner)
			_ = raw.Close()
			return
		}
	}
	tlsConn := tls.Server(raw, s.tlsCfg)
	_ = raw.SetDeadline(time.Now().Add(30 * time.Second))
	if err := tlsConn.Handshake(); err != nil {
		// TLS 握手失败（HTTPS 扫描/协议错）须上报探针，否则仅 TCP Accept 路径能记事件。
		if s.cfg.Probe != nil {
			s.cfg.Probe.OnTransportReadError(remote, err)
		}
		_ = raw.Close()
		return
	}
	_ = raw.SetDeadline(time.Time{})
	conn := AcceptConn(tlsConn, s.cfg, nil, func(err error) {
		logger.Debug("peer disconnected: %v", err)
	})
	if s.onConn != nil {
		s.onConn(conn)
	}
}

// Close 关闭监听器。
func (s *Server) Close() error {
	return s.listener.Close()
}

// Addr 返回监听器地址。
func (s *Server) Addr() net.Addr {
	return s.listener.Addr()
}

// discardConn 测试用：读尽并关闭。
func discardConn(c net.Conn) {
	_, _ = io.Copy(io.Discard, c)
	_ = c.Close()
}
