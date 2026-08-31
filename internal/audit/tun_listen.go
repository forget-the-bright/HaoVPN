package audit

import "strconv"

// LogTunAdminListen 管理口绑定 TUN 网关 IP 时写审计与启动日志提示。
//
// tunIP 为 VPN 网关地址；管理 API 为明文 HTTP，已连接 VPN 的账号可尝试访问登录页。
func LogTunAdminListen(auditLog *Logger, tunIP string, port int) {
	if auditLog == nil || tunIP == "" {
		return
	}
	auditLog.Log(nil, "management_tun_listen", "system", nil, "", map[string]string{
		"tun_ip":  tunIP,
		"port":    strconv.Itoa(port),
		"message": "管理口已绑定 VPN 网关 IP（明文 HTTP）；VPN 内用户可访问，风险自担；可设 api.listen_tun: false 关闭",
	})
}
