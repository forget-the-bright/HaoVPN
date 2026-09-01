package winnet_test

import (
	"testing"

	"haovpn/internal/config"
	"haovpn/internal/winnet"
)

// TestClientWindowsDefaults UseIPHelper 默认 true。
func TestClientWindowsDefaults(t *testing.T) {
	var w config.ClientWindowsSection
	if !w.UseIPHelperEnabled() {
		t.Fatal("默认 use_ip_helper 应为 true")
	}
	f := false
	w.UseIPHelper = &f
	if w.UseIPHelperEnabled() {
		t.Fatal("显式 false 应关闭")
	}
}

// TestConfigureOptions 注入开关可读写。
func TestConfigureOptions(t *testing.T) {
	winnet.Configure(winnet.Options{UseIPHelper: false})
	if winnet.UseIPHelperEnabled() {
		t.Fatal("应为 false")
	}
	winnet.Configure(winnet.Options{UseIPHelper: true})
	if !winnet.UseIPHelperEnabled() {
		t.Fatal("应为 true")
	}
}
