package api

import (
	"fmt"
	"net/http"

	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/persist"
	"haovpn/internal/vpnaccount"
)

// loadExportAccount 加载可导出的 VPN 账号并解密私钥。
//
// 参数：id — users.id。
// 返回：User、明文私钥；无 VPN 配置或解密失败时 error（供 handler 映射 404/500）。
func (s *Server) loadExportAccount(id int64) (*persist.User, string, error) {
	u, err := s.store.GetUserByID(id)
	if err != nil || !u.HasVPN() {
		return nil, "", fmt.Errorf("账号不存在或无 VPN 配置")
	}
	plainKey, err := vpnaccount.OpenAccountPrivateKey(u, s.keyEnc)
	if err != nil {
		return nil, "", fmt.Errorf("私钥解密失败")
	}
	return u, plainKey, nil
}

// handleUserExportZip 下载账号客户端 ZIP（含 yaml + 证书等）。
//
// 副作用：写审计 config_export；响应经 writeAttachment。
func (s *Server) handleUserExportZip(w http.ResponseWriter, r *http.Request, id int64, se auth.SessionEntry) {
	u, plainKey, err := s.loadExportAccount(id)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, err.Error())
		return
	}
	zipBytes, err := buildAccountExportZip(s.cfg, u, plainKey, s.serverPK)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit.Log(&se.UserID, "config_export", "user", &id, clientIP(r), map[string]string{"format": "zip"})
	writeAttachment(w, "application/zip", fmt.Sprintf("haovpn-client-%s.zip", u.Username), zipBytes)
}

// handleUserExportYAML 下载仅 client.yaml（不含私钥落盘，策略握手下发）。
//
// 副作用：写审计 config_export；YAML 由 config.BuildClientExportYAML 生成。
func (s *Server) handleUserExportYAML(w http.ResponseWriter, r *http.Request, id int64, se auth.SessionEntry) {
	u, _, err := s.loadExportAccount(id)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, err.Error())
		return
	}
	caFile := config.ResolveServerCertPath(s.cfg)
	yaml := config.BuildClientExportYAML(s.cfg.Server.Listen, u.Username, caFile, s.cfg.VPN.MTU)
	s.audit.Log(&se.UserID, "config_export", "user", &id, clientIP(r), nil)
	writeAttachment(w, "application/x-yaml", fmt.Sprintf("client-%s.yaml", u.Username), []byte(yaml))
}
