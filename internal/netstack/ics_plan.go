package netstack

import (
	"fmt"
	"strings"
)

// ICSLANBinding 某 LAN 网段解析到的出站网卡（供决策与提示，无系统副作用）。
type ICSLANBinding struct {
	CIDR   string // 如 192.168.31.0/24
	IfName string // 出站网卡名；空表示解析失败
}

// ICSOutboundPlan ICS 多网段决策结果：以第一条有效绑定的网卡为准。
//
// 同网卡网段一并生效；异网卡网段跳过（平台只能一对共享）。
type ICSOutboundPlan struct {
	PrimaryIf string         // 生效的出站网卡；无有效绑定时为空
	Active    []ICSLANBinding // 与 PrimaryIf 相同的网段
	Skipped   []ICSLANBinding // 其它网卡或未解析成功的网段
}

// PlanICSByOutbound 按列表顺序取第一条有效出站网卡为 Primary，归类 active/skipped。
//
// 参数：bindings — 已解析的 (CIDR→IfName)；顺序即产品「第一条」语义。
// 返回：PrimaryIf 空表示没有任何可用绑定。
func PlanICSByOutbound(bindings []ICSLANBinding) ICSOutboundPlan {
	return PlanICSByOutboundPreferred("", bindings)
}

// PlanICSByOutboundPreferred 在 preferred 非空时只认该网卡：同名进 Active，其它进 Skipped。
//
// preferred 为空时与「第一条有效网卡」语义相同。无任何 Active 时 PrimaryIf 为空。
func PlanICSByOutboundPreferred(preferred string, bindings []ICSLANBinding) ICSOutboundPlan {
	preferred = strings.TrimSpace(preferred)
	var plan ICSOutboundPlan
	if preferred != "" {
		for _, b := range bindings {
			cidr := strings.TrimSpace(b.CIDR)
			ifn := strings.TrimSpace(b.IfName)
			if cidr == "" {
				continue
			}
			item := ICSLANBinding{CIDR: cidr, IfName: ifn}
			if ifn != "" && strings.EqualFold(ifn, preferred) {
				plan.Active = append(plan.Active, item)
			} else {
				plan.Skipped = append(plan.Skipped, item)
			}
		}
		if len(plan.Active) > 0 {
			plan.PrimaryIf = preferred
		}
		return plan
	}
	for _, b := range bindings {
		cidr := strings.TrimSpace(b.CIDR)
		ifn := strings.TrimSpace(b.IfName)
		item := ICSLANBinding{CIDR: cidr, IfName: ifn}
		if cidr == "" {
			continue
		}
		if ifn == "" {
			plan.Skipped = append(plan.Skipped, item)
			continue
		}
		if plan.PrimaryIf == "" {
			plan.PrimaryIf = ifn
			plan.Active = append(plan.Active, item)
			continue
		}
		if strings.EqualFold(ifn, plan.PrimaryIf) {
			plan.Active = append(plan.Active, item)
		} else {
			plan.Skipped = append(plan.Skipped, item)
		}
	}
	return plan
}

// FormatICSLocalLANsHint 生成用户可见中文提示（日志与 GUI 共用）。
//
// 有 Skipped 时返回完整多行说明；仅 Active、无跳过时返回短 Info；无 Primary 返回空。
func FormatICSLocalLANsHint(plan ICSOutboundPlan) string {
	if plan.PrimaryIf == "" {
		return ""
	}
	activeCIDRs := joinBindingCIDRs(plan.Active)
	if len(plan.Skipped) == 0 {
		if len(plan.Active) <= 1 {
			return ""
		}
		return fmt.Sprintf(
			"ICS 已按网卡「%s」启用一次，下列网段将一并出口：%s。",
			plan.PrimaryIf, activeCIDRs,
		)
	}
	var skippedParts []string
	for _, s := range plan.Skipped {
		if strings.TrimSpace(s.IfName) == "" {
			skippedParts = append(skippedParts, fmt.Sprintf("%s（未能解析出站网卡）", s.CIDR))
		} else {
			skippedParts = append(skippedParts, fmt.Sprintf("%s（出站网卡「%s」）", s.CIDR, s.IfName))
		}
	}
	return strings.Join([]string{
		"本机作经由出口时，Windows 连接共享（ICS）同一时间只能共享一块网卡。",
		fmt.Sprintf("已启用：网卡「%s」← 网段 %s（与第一条同网卡，可同时生效）。", plan.PrimaryIf, activeCIDRs),
		fmt.Sprintf("无法同时生效：%s。", strings.Join(skippedParts, "；")),
		"处理建议：把需访问的网段接到与第一条相同的网卡/路由；或改用支持 WinNAT 的 Windows 专业版；或在配置中指定单一 outbound_interface。",
	}, "\n")
}

func joinBindingCIDRs(bs []ICSLANBinding) string {
	parts := make([]string, 0, len(bs))
	for _, b := range bs {
		if c := strings.TrimSpace(b.CIDR); c != "" {
			parts = append(parts, c)
		}
	}
	return strings.Join(parts, "、")
}
