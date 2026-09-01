//go:build windows

package winnet

import (
	"context"
	"fmt"
	"strings"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/platform"
)

// PreferVPNAfterSoftIPReplace 在线软换 VPN IP 后的轻量 PreferVPN（ICS 仍在、Replace 已做 /32）。
//
// 主路径：iphlp 清默认路由 → iphlp SkipAsSource（noop/iphlp）；失败或无 137 才 PS/完整 PreferVPN。
func PreferVPNAfterSoftIPReplace(ctx context.Context, configName string, ifIndex int, vpnIP string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	vpnIP = strings.TrimSpace(vpnIP)
	if configName == "" || vpnIP == "" || ifIndex <= 0 {
		return fmt.Errorf("PreferVPNAfterSoftIPReplace: 参数无效")
	}
	start := time.Now()

	if ents, err := ListUnicastIPv4OnIfIndex(ifIndex); err == nil {
		for _, e := range ents {
			if e.IP.String() == vpnIP && e.PrefixLen != 32 {
				logger.Warn("prefer_vpn_light vpn=%s prefix=%d want=32", vpnIP, e.PrefixLen)
			}
		}
	}

	if _, err := DeleteDefaultRouteOnInterface(ifIndex, ScrubDefaultRouteFast); err != nil {
		logger.Warn("prefer_vpn_light scrub: %v", err)
	}

	method, err := ApplyPreferVPNSkipAsSource(ifIndex, vpnIP)
	if err == nil && (method == "noop" || method == "iphlp") {
		logger.Info("prefer_vpn_light method=%s elapsed=%s", method, time.Since(start))
		return nil
	}
	if err != nil {
		logger.Warn("prefer_vpn_light iphlp_skipas fail: %v", err)
	}

	has137 := false
	if ents, err := ListUnicastIPv4OnIfIndex(ifIndex); err == nil {
		for _, e := range ents {
			if netutil.IPv4IsICSPrivate(e.IP) {
				has137 = true
				break
			}
		}
	}
	if !has137 {
		logger.Info("prefer_vpn_light method=full_fallback reason=no_137 elapsed=%s", time.Since(start))
		return PreferVPNSourceWithICSContext(ctx, configName, vpnIP)
	}

	// PS fallback：已知 ifIndex，不跑 AssignAdapterIf
	ps := PSAssignAdapterAndSkipAsSourceOnly(vpnIP, ifIndex)
	out, psErr := RunPSOneShotContext(ctx, ps)
	logger.Info("prefer_vpn_light method=ps elapsed=%s err=%v", time.Since(start), psErr)
	if psErr != nil {
		return platform.CommandOutputError("PreferVPNAfterSoftIPReplace", out, psErr)
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ics_src_diag ") {
			logger.Info("windows: %s", line)
		}
	}
	return nil
}
