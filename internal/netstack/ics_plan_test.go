package netstack

import (
	"strings"
	"testing"
)

// TestPlanICSByOutboundSameNIC 同网卡多段全部 active。
func TestPlanICSByOutboundSameNIC(t *testing.T) {
	plan := PlanICSByOutbound([]ICSLANBinding{
		{CIDR: "192.168.31.0/24", IfName: "以太网"},
		{CIDR: "192.168.32.0/24", IfName: "以太网"},
	})
	if plan.PrimaryIf != "以太网" || len(plan.Active) != 2 || len(plan.Skipped) != 0 {
		t.Fatalf("plan=%+v", plan)
	}
	hint := FormatICSLocalLANsHint(plan)
	if hint == "" || !strings.Contains(hint, "以太网") || !strings.Contains(hint, "192.168.31.0/24") {
		t.Fatalf("hint=%q", hint)
	}
	if strings.Contains(hint, "无法同时生效") {
		t.Fatalf("同网卡不应含无法同时生效: %q", hint)
	}
}

// TestPlanICSByOutboundMultiNIC 异网卡：仅首网卡生效，文案含跳过。
func TestPlanICSByOutboundMultiNIC(t *testing.T) {
	plan := PlanICSByOutbound([]ICSLANBinding{
		{CIDR: "192.168.31.0/24", IfName: "以太网"},
		{CIDR: "192.168.32.0/24", IfName: "以太网"},
		{CIDR: "192.168.10.0/24", IfName: "WLAN"},
	})
	if plan.PrimaryIf != "以太网" || len(plan.Active) != 2 || len(plan.Skipped) != 1 {
		t.Fatalf("plan=%+v", plan)
	}
	if plan.Skipped[0].CIDR != "192.168.10.0/24" {
		t.Fatalf("skipped=%+v", plan.Skipped)
	}
	hint := FormatICSLocalLANsHint(plan)
	for _, want := range []string{"ICS", "以太网", "WLAN", "192.168.10.0/24", "无法同时生效", "WinNAT"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("hint 缺 %q: %q", want, hint)
		}
	}
}

// TestPlanICSByOutboundUnresolved 未解析网卡进 skipped。
func TestPlanICSByOutboundUnresolved(t *testing.T) {
	plan := PlanICSByOutbound([]ICSLANBinding{
		{CIDR: "10.0.0.0/24", IfName: "Eth0"},
		{CIDR: "10.1.0.0/24", IfName: ""},
	})
	if len(plan.Active) != 1 || len(plan.Skipped) != 1 {
		t.Fatalf("plan=%+v", plan)
	}
	hint := FormatICSLocalLANsHint(plan)
	if !strings.Contains(hint, "未能解析出站网卡") || !strings.Contains(hint, "10.1.0.0/24") {
		t.Fatalf("hint=%q", hint)
	}
}

// TestFormatICSLocalLANsHintSingleNoSkip 单网段无跳过 → 空提示。
func TestFormatICSLocalLANsHintSingleNoSkip(t *testing.T) {
	plan := PlanICSByOutbound([]ICSLANBinding{{CIDR: "192.168.1.0/24", IfName: "LAN"}})
	if FormatICSLocalLANsHint(plan) != "" {
		t.Fatal("单段无冲突应空提示")
	}
}

// TestPlanICSByOutboundPreferred 指定 outbound_interface：只认该网卡。
func TestPlanICSByOutboundPreferred(t *testing.T) {
	plan := PlanICSByOutboundPreferred("以太网", []ICSLANBinding{
		{CIDR: "192.168.31.0/24", IfName: "以太网"},
		{CIDR: "192.168.10.0/24", IfName: "WLAN"},
		{CIDR: "192.168.32.0/24", IfName: "以太网"},
	})
	if plan.PrimaryIf != "以太网" || len(plan.Active) != 2 || len(plan.Skipped) != 1 {
		t.Fatalf("plan=%+v", plan)
	}
	if plan.Skipped[0].CIDR != "192.168.10.0/24" {
		t.Fatalf("skipped=%+v", plan.Skipped)
	}
	hint := FormatICSLocalLANsHint(plan)
	if !strings.Contains(hint, "WLAN") || !strings.Contains(hint, "无法同时生效") {
		t.Fatalf("hint=%q", hint)
	}
}

// TestPlanICSByOutboundPreferredNone 指定网卡但无一匹配 → Primary 空。
func TestPlanICSByOutboundPreferredNone(t *testing.T) {
	plan := PlanICSByOutboundPreferred("以太网", []ICSLANBinding{
		{CIDR: "192.168.10.0/24", IfName: "WLAN"},
	})
	if plan.PrimaryIf != "" || len(plan.Active) != 0 || len(plan.Skipped) != 1 {
		t.Fatalf("plan=%+v", plan)
	}
}
