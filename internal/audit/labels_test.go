package audit_test

import (
	"testing"

	"haovpn/internal/audit"
)

func TestActionLabelKnown(t *testing.T) {
	if got := audit.ActionLabel("login"); got != "登录" {
		t.Fatalf("login: %q", got)
	}
	if got := audit.ActionLabel("peers_apply"); got != "应用托管路由" {
		t.Fatalf("peers_apply: %q", got)
	}
}

func TestActionLabelUnknownPassthrough(t *testing.T) {
	if got := audit.ActionLabel("unknown_action_xyz"); got != "" {
		t.Fatalf("unknown should be empty zh, got %q", got)
	}
	if got := audit.FormatActionZH("unknown_action_xyz"); got != "unknown_action_xyz" {
		t.Fatalf("FormatActionZH unknown: %q", got)
	}
}

func TestFormatActionZH(t *testing.T) {
	if got := audit.FormatActionZH("login"); got != "login（登录）" {
		t.Fatalf("got %q", got)
	}
}

func TestTargetTypeLabel(t *testing.T) {
	if got := audit.TargetTypeLabel("user"); got != "用户" {
		t.Fatalf("user: %q", got)
	}
	if got := audit.TargetTypeLabel("peer_route"); got != "托管路由" {
		t.Fatalf("peer_route: %q", got)
	}
}
