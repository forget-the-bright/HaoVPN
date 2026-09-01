package netstack

// NATSetupOutcome 平台 NAT Setup 结果（Windows ICS 时带回决策，供注册表纠正与提示）。
type NATSetupOutcome struct {
	UsedICS bool            // 是否启用了 ICS（非 WinNAT / iptables）
	Plan    ICSOutboundPlan // UsedICS 时有效：Active 生效、Skipped 不生效
}
