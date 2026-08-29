package api

import (
	"errors"
	"fmt"
	"net/http"

	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/persist"
	"haovpn/internal/vpnaccount"
)

// loadExportAccount 加载可导出的 VPN 账号（不解密私钥：导出包仅含 client.yaml + 证书，密钥由握手下发）。
//
// 参数：id — users.id。
// 返回：User；无 VPN 配置时 ErrAccountNotFound。
func (s *Server) loadExportAccount(id int64) (*persist.User, error) {
	u, err := s.store.GetUserByID(id)
	if err != nil {
		return nil, err
	}
	if u == nil || !u.HasVPN() {
		return nil, vpnaccount.ErrAccountNotFound
	}
	return u, nil
}

// handleUserExportZip 下载账号客户端 ZIP（含 yaml + 证书等）。
//
// 副作用：写审计 config_export；响应经 writeAttachment。
func (s *Server) handleUserExportZip(w http.ResponseWriter, r *http.Request, id int64, se auth.SessionEntry) {
	u, err := s.loadExportAccount(id)
	if err != nil {
		if errors.Is(err, vpnaccount.ErrAccountNotFound) {
			writeAPIError(w, http.StatusNotFound, err.Error())
			return
		}
		writeInternalError(w, err)
		return
	}
	zipBytes, err := buildAccountExportZip(s.cfg, u)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	s.audit.Log(&se.UserID, "config_export", "user", &id, s.clientIP(r), map[string]string{"format": "zip"})
	writeAttachment(w, "application/zip", fmt.Sprintf("haovpn-client-%s.zip", u.Username), zipBytes)
}

// handleUserExportYAML 下载仅 client.yaml（不含私钥落盘，策略握手下发）。
//
// 副作用：写审计 config_export；YAML 由 config.BuildClientExportYAML 生成。
func (s *Server) handleUserExportYAML(w http.ResponseWriter, r *http.Request, id int64, se auth.SessionEntry) {
	u, err := s.loadExportAccount(id)
	if err != nil {
		if errors.Is(err, vpnaccount.ErrAccountNotFound) {
			writeAPIError(w, http.StatusNotFound, err.Error())
			return
		}
		writeInternalError(w, err)
		return
	}
	caFile := config.ResolveServerCertPath(s.cfg)
	yaml := config.BuildClientExportYAML(s.cfg.Server.Listen, u.Username, caFile, s.cfg.VPN.MTU)
	s.audit.Log(&se.UserID, "config_export", "user", &id, s.clientIP(r), nil)
	writeAttachment(w, "application/x-yaml", fmt.Sprintf("client-%s.yaml", u.Username), []byte(yaml))
}

