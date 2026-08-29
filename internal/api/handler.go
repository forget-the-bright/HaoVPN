package api

import (
	"net/http"
	"sync"
	"time"

	"haovpn/internal/audit"
	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/logstore"
	"haovpn/internal/logger"
	"haovpn/internal/persist"
	"haovpn/internal/probedefense"
	"haovpn/internal/security"
	"haovpn/internal/sessionmgr"
	"haovpn/internal/vpnaccount"
)

// Server 管理 Web API 与页面路由的 HTTP 服务实例。
type Server struct {
	cfg        *config.ServerConfig
	store      *persist.Store
	auth       *auth.Service
	audit      *audit.Logger
	sessions   *sessionmgr.Manager
	keyEnc     *security.KeyEnc
	vpnSvc     *vpnaccount.Service
	probeGuard *probedefense.Guard
	startedAt  time.Time
	serverPK   string
	mux        *http.ServeMux
	httpServer *http.Server
	tunOK      bool
	natOK      bool
	logStore   *logstore.Store

	// peerDirty 托管路由/互访变更后待「应用生效」的账号；all=true 表示须踢全部 VPN 账号。
	peerDirtyMu  sync.Mutex
	peerDirtyAll bool
	peerDirtyIDs map[int64]struct{}
}

// NewServer 创建 API 服务实例并完成路由注册。
func NewServer(
	cfg *config.ServerConfig,
	store *persist.Store,
	authSvc *auth.Service,
	auditLog *audit.Logger,
	sessMgr *sessionmgr.Manager,
	vpnSvc *vpnaccount.Service,
	keyEnc *security.KeyEnc,
	startedAt time.Time,
	serverTunnelPublicKey string,
) *Server {
	s := &Server{
		cfg:          cfg,
		store:        store,
		auth:         authSvc,
		audit:        auditLog,
		sessions:     sessMgr,
		vpnSvc:       vpnSvc,
		keyEnc:       keyEnc,
		startedAt:    startedAt,
		serverPK:     serverTunnelPublicKey,
		mux:          http.NewServeMux(),
		peerDirtyIDs: map[int64]struct{}{},
	}
	s.routes()
	return s
}

// SetLogStore 注入结构化历史日志库（可选）。
func (s *Server) SetLogStore(ls *logstore.Store) { s.logStore = ls }

// SetProbeGuard 注入探针防御 Guard（可选；供安全事件/封禁 API）。
func (s *Server) SetProbeGuard(g *probedefense.Guard) { s.probeGuard = g }

// SetVPNService 注入或替换 VPN 账号领域服务。
func (s *Server) SetVPNService(v *vpnaccount.Service) { s.vpnSvc = v }

// SetDataplaneHealth 设置 TUN/NAT 数据面就绪状态。
func (s *Server) SetDataplaneHealth(tunOK, natOK bool) {
	s.tunOK = tunOK
	s.natOK = natOK
}

// Handler 返回带安全中间件包装的 HTTP 处理器。
func (s *Server) Handler() http.Handler { return s.withMiddleware(s.mux) }

// Listen 在指定地址启动单监听 HTTP 服务。
func (s *Server) Listen(addr string) error {
	s.httpServer = &http.Server{Addr: addr, Handler: s.withMiddleware(s.mux)}
	logger.Info("管理 API 监听: %s", addr)
	return s.httpServer.ListenAndServe()
}

// Close 优雅关闭由 Listen 创建的 HTTP 服务。
func (s *Server) Close() error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Close()
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range security.SecurityHeaders() {
			w.Header().Set(k, v)
		}
		next.ServeHTTP(w, r)
	})
}
