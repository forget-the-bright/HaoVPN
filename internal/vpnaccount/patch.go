package vpnaccount

import (
	"haovpn/internal/netutil"
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
