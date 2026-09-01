package netstack

import (
	"context"
	"errors"
	"net"
	"testing"
)

// TestSetupAbortNotForwardOnlySuccess ctx 取消须返回 error，禁止 forward_only 吞成「无 SNAT 成功」。
//
// 回归：现场日志曾出现 ICS 已取消后仍 via_exit_setup ok snat=false + 误装路由。
// Setup(ctx) 取代旧 AbortCtx 字段，避免生命周期塞进 Config。
func TestSetupAbortNotForwardOnlySuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	st := New(Config{
		TunName:     "haovpn_client",
		TunIP:       net.ParseIP("10.88.0.2"),
		VPNSubnet:   "10.88.0.0/24",
		LanCIDRs:    []string{"192.168.31.0/24"},
		ForwardOnly: true,
		Enabled:     true,
	})
	err := st.Setup(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v want Canceled（不得 nil/forward_only 成功）", err)
	}
	if st.SNATEnabled() {
		t.Fatal("abort 后不应 snatEnabled")
	}
}
