package tunnel

import (
	"errors"
	"fmt"

	"haovpn/internal/netutil"
)

// ErrSourceDenied 隧道来源不在 tunnel_allowed_source_ips；探针签名 source_deny。
var ErrSourceDenied = errors.New("隧道来源 IP 不在 tunnel_allowed_source_ips 白名单内")

// CheckTunnelSourceIP 校验隧道连接来源 IP 是否在白名单内；allowed 为空表示不限制。
//
// 参数：
//   remoteAddr — transport.Conn.RemoteAddr()，可为 host:port。
//   allowed — tunnel_allowed_source_ips 配置；支持 CIDR 与单 IP。
// 返回：不在白名单时为 ErrSourceDenied（可 errors.Is）；地址无法解析时为解析错误。
// 关联：匹配算法与 probedefense.Guard.AllowSourceIP 相同（均走 netutil.IPMatchesRules），
// 握手阶段再检一次以防 Accept 未挂 Probe 时漏拦；Accept 侧白名单仅在探针 Enabled 时生效。
func CheckTunnelSourceIP(remoteAddr string, allowed []string) error {
	if len(allowed) == 0 {
		return nil
	}
	ip, err := netutil.ParseHostIP(remoteAddr)
	if err != nil {
		return err
	}
	if netutil.IPMatchesRules(ip, allowed) {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrSourceDenied, ip)
}
