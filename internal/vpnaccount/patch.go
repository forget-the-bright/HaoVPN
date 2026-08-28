package vpnaccount

import (
	"fmt"
	"strings"

	"haovpn/internal/netutil"
	"haovpn/internal/persist"
)

// VPNPatchResult ApplyVPNPatch 成功后的策略版本与生效字段（供 API JSON 响应）。
type VPNPatchResult struct {
	PolicyVer int
	NewIP     string
	NewMode   string
}

// ApplyVPNPatch 完整执行 VPN 策略 PATCH：校验、递增 policy_ver、写库、踢线。
//
// 参数：userID — users.id；in — 与 Web PATCH JSON 一致。
// 返回：VPNPatchResult 与 err；用户不存在、校验失败或 DB 错误时 err 非 nil。
// 副作用：IncrementPolicyVer、UpdateVPNFields；OnKickUser 使新策略生效。
func (s *Service) ApplyVPNPatch(userID int64, in VPNPatchInput) (VPNPatchResult, error) {
	u, err := s.Store.GetUserByID(userID)
	if err != nil {
		return VPNPatchResult{}, err
	}
	if u == nil || !u.HasVPN() {
		return VPNPatchResult{}, ErrAccountNotFound
	}
	plan, err := s.PlanVPNPatch(u, in)
	if err != nil {
		return VPNPatchResult{}, err
	}
	newIP := plan.NewIP
	if norm, err := netutil.NormalizeIPv4(newIP); err == nil && newIP != "" {
		newIP = norm
	}
	pv, err := s.Store.IncrementPolicyVer(userID)
	if err != nil {
		return VPNPatchResult{}, err
	}
	if err := s.Store.UpdateVPNFields(userID, newIP, plan.AllowedIPs, plan.NewMode, plan.IPLeaseSec, pv); err != nil {
		return VPNPatchResult{}, err
	}
	if s.OnKickUser != nil {
		s.OnKickUser(userID)
	}
	return VPNPatchResult{PolicyVer: pv, NewIP: newIP, NewMode: plan.NewMode}, nil
}

// VPNPatchInput Web PATCH /users/{id}/vpn 的请求体（与 api JSON 字段一致）。
type VPNPatchInput struct {
	AllowedIPs *[]string
	IPMode     string
	IPLeaseSec int
	VPNIP      *string
}

// VPNPatchPlan 校验通过后的策略变更计划（供 api 写库与踢线）。
type VPNPatchPlan struct {
	NewIP      string
	NewMode    string
	AllowedIPs []string
	IPLeaseSec int
}

// PlanVPNPatch 解析 IP 模式与 vpn_ip 变更，并在需要时执行 fixed 模式池重绑。
//
// 参数：u — 当前账号；in — PATCH 请求字段。
// 返回：VPNPatchPlan 与 err；动态模式禁止指定 vpn_ip。
// 副作用：可能调用 RebindFixedVPNIP / ReleaseFixedVPNIP（fixed↔dynamic 切换）。
func (s *Service) PlanVPNPatch(u *persist.User, in VPNPatchInput) (VPNPatchPlan, error) {
	if u == nil || !u.HasVPN() {
		return VPNPatchPlan{}, ErrAccountNotFound
	}
	oldMode, oldIP := u.IPMode, u.VPNIP
	if oldMode == "" {
		oldMode = persist.IPModeFixed
	}
	newMode := oldMode
	if in.IPMode != "" {
		newMode = in.IPMode
	}
	leaseSec := u.IPLeaseSec
	if in.IPLeaseSec > 0 {
		leaseSec = in.IPLeaseSec
	}
	allowed := u.AllowedIPs
	if in.AllowedIPs != nil {
		if err := ValidateAllowedIPs(*in.AllowedIPs); err != nil {
			return VPNPatchPlan{}, err
		}
		allowed = *in.AllowedIPs
	}

	newIP := oldIP
	if in.VPNIP != nil {
		req := strings.TrimSpace(*in.VPNIP)
		switch newMode {
		case persist.IPModeDynamicSession, persist.IPModeDynamicLease:
			if req != "" {
				return VPNPatchPlan{}, fmt.Errorf("动态 IP 模式不可指定 VPN IP")
			}
			newIP = ""
		case persist.IPModeFixed:
			if req == "" {
				return VPNPatchPlan{}, fmt.Errorf("fixed 模式须指定 VPN IP，或省略 vpn_ip 字段保持不变")
			}
			newIP = req
		default:
			return VPNPatchPlan{}, fmt.Errorf("未知 ip_mode")
		}
	} else if newMode != oldMode {
		if newMode == persist.IPModeFixed && oldIP == "" {
			return VPNPatchPlan{}, fmt.Errorf("切到 fixed 须指定 vpn_ip")
		}
		if newMode == persist.IPModeDynamicSession || newMode == persist.IPModeDynamicLease {
			newIP = ""
		}
	}

	wasFixed := oldMode == persist.IPModeFixed
	willFixed := newMode == persist.IPModeFixed
	if wasFixed && !willFixed {
		s.ReleaseFixedVPNIP(u.ID, oldIP)
	} else if willFixed {
		if !wasFixed {
			if err := s.RebindFixedVPNIP(u.ID, "", newIP); err != nil {
				return VPNPatchPlan{}, err
			}
		} else if newIP != oldIP {
			if err := s.RebindFixedVPNIP(u.ID, oldIP, newIP); err != nil {
				return VPNPatchPlan{}, err
			}
		}
	}

	return VPNPatchPlan{
		NewIP:      newIP,
		NewMode:    newMode,
		AllowedIPs: allowed,
		IPLeaseSec: leaseSec,
	}, nil
}
