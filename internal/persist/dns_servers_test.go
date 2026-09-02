package persist_test

import (
	"path/filepath"
	"strings"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/persist"
)

// TestDNSServerCRUDAndResolve 覆盖创建、排除、按账号解析、config 不可删、YAML seed。
func TestDNSServerCRUDAndResolve(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "dns.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	hash, _ := auth.HashPassword("pass12345!")
	mk := func(name, vip string) int64 {
		id, err := store.CreateVPNAccount(persist.User{
			Username: name, PasswordHash: hash, PublicKey: "pk-" + name, PrivateKeyEnc: "sk",
			VPNIP: vip, AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	home := mk("home", "10.88.0.2")
	corp := mk("corp", "10.88.0.3")

	// YAML seed：两条 config，all 绑定
	added, kept, removed, err := store.SyncConfigDNSServers([]string{"10.10.6.51", "192.168.3.1", "10.10.6.51"})
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 || kept != 0 || removed != 0 {
		t.Fatalf("seed1 added=%d kept=%d removed=%d", added, kept, removed)
	}
	// 幂等
	added, kept, removed, err = store.SyncConfigDNSServers([]string{"10.10.6.51", "192.168.3.1"})
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 || kept != 2 || removed != 0 {
		t.Fatalf("seed2 added=%d kept=%d removed=%d", added, kept, removed)
	}

	list, err := store.ListDNSServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("list len=%d", len(list))
	}
	var corpDNSID int64
	for _, d := range list {
		if d.DNSIP == "10.10.6.51" {
			corpDNSID = d.ID
			if !d.IsConfigSource() || len(d.MemberUserIDs) != 1 || d.MemberUserIDs[0] != 0 {
				t.Fatalf("config row bad: %+v", d)
			}
		}
		if _, err := store.DeleteDNSServer(d.ID); err == nil {
			t.Fatal("config delete should fail")
		}
	}

	// 排除家里账号
	if err := store.ReplaceDNSServerExcludes(corpDNSID, []int64{home}); err != nil {
		t.Fatal(err)
	}
	ipsHome, err := store.ListDNSIPsForUser(home)
	if err != nil {
		t.Fatal(err)
	}
	ipsCorp, err := store.ListDNSIPsForUser(corp)
	if err != nil {
		t.Fatal(err)
	}
	if containsIP(ipsHome, "10.10.6.51") {
		t.Fatalf("home should exclude corp dns: %v", ipsHome)
	}
	if !containsIP(ipsCorp, "10.10.6.51") || !containsIP(ipsCorp, "192.168.3.1") {
		t.Fatalf("corp should get both: %v", ipsCorp)
	}
	if !containsIP(ipsHome, "192.168.3.1") {
		t.Fatalf("home should still get other dns: %v", ipsHome)
	}

	// 手工指定仅 corp
	manID, err := store.CreateDNSServer("10.20.0.1", "公司备用", []int64{corp}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ipsHome, _ = store.ListDNSIPsForUser(home)
	ipsCorp, _ = store.ListDNSIPsForUser(corp)
	if containsIP(ipsHome, "10.20.0.1") {
		t.Fatal("home should not get manual specific dns")
	}
	if !containsIP(ipsCorp, "10.20.0.1") {
		t.Fatal("corp should get manual dns")
	}

	// 删账号级联排除
	if err := store.DeleteUser(home); err != nil {
		t.Fatal(err)
	}
	d, err := store.GetDNSServer(corpDNSID)
	if err != nil || d == nil {
		t.Fatal(err)
	}
	for _, e := range d.ExcludeUserIDs {
		if e == home {
			t.Fatal("exclude should clear on DeleteUser")
		}
	}

	if _, err := store.DeleteDNSServer(manID); err != nil {
		t.Fatal(err)
	}
	// YAML 去掉一条
	_, _, removed, err = store.SyncConfigDNSServers([]string{"192.168.3.1"})
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed=%d want 1", removed)
	}
}

func containsIP(list []string, ip string) bool {
	for _, x := range list {
		if x == ip {
			return true
		}
	}
	return false
}

// TestDNSSeedPreservesExcludes YAML 再 Sync 不得清掉已配排除。
func TestDNSSeedPreservesExcludes(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "dns-ex.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hash, _ := auth.HashPassword("pass12345!")
	home, err := store.CreateVPNAccount(persist.User{
		Username: "home", PasswordHash: hash, PublicKey: "pk", PrivateKeyEnc: "sk",
		VPNIP: "10.88.0.2", AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.SyncConfigDNSServers([]string{"10.10.6.51"}); err != nil {
		t.Fatal(err)
	}
	list, _ := store.ListDNSServers()
	if len(list) != 1 {
		t.Fatalf("len=%d", len(list))
	}
	id := list[0].ID
	if err := store.ReplaceDNSServerExcludes(id, []int64{home}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.SyncConfigDNSServers([]string{"10.10.6.51"}); err != nil {
		t.Fatal(err)
	}
	d, err := store.GetDNSServer(id)
	if err != nil || d == nil {
		t.Fatal(err)
	}
	if len(d.ExcludeUserIDs) != 1 || d.ExcludeUserIDs[0] != home {
		t.Fatalf("excludes lost: %+v", d.ExcludeUserIDs)
	}
}

// TestDNSConfigRejectsReplaceMembers config 源禁止改包含集。
func TestDNSConfigRejectsReplaceMembers(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "dns-cfg.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hash, _ := auth.HashPassword("pass12345!")
	u, _ := store.CreateVPNAccount(persist.User{
		Username: "u", PasswordHash: hash, PublicKey: "pk", PrivateKeyEnc: "sk",
		VPNIP: "10.88.0.2", AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if _, _, _, err := store.SyncConfigDNSServers([]string{"10.10.6.51"}); err != nil {
		t.Fatal(err)
	}
	list, _ := store.ListDNSServers()
	if err := store.ReplaceDNSServerMembers(list[0].ID, []int64{u}); err == nil {
		t.Fatal("config members replace should fail")
	}
}

// TestDNSManualSameIPAsYAMLSkipped Manual 已占 IP 时 YAML seed 跳过。
func TestDNSManualSameIPAsYAMLSkipped(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "dns-clash.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hash, _ := auth.HashPassword("pass12345!")
	u, _ := store.CreateVPNAccount(persist.User{
		Username: "u", PasswordHash: hash, PublicKey: "pk", PrivateKeyEnc: "sk",
		VPNIP: "10.88.0.2", AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	manID, err := store.CreateDNSServer("10.10.6.51", "手工", []int64{u}, nil)
	if err != nil {
		t.Fatal(err)
	}
	added, kept, removed, err := store.SyncConfigDNSServers([]string{"10.10.6.51"})
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 {
		t.Fatalf("不应新增 config 行 overlapping manual, added=%d kept=%d removed=%d", added, kept, removed)
	}
	d, _ := store.GetDNSServer(manID)
	if d == nil || d.Source != persist.DNSSourceManual {
		t.Fatalf("manual 应仍在: %+v", d)
	}
}

// TestDeleteUserRemovesEmptyManualDNS 删唯一成员后手工 DNS 行应删除。
func TestDeleteUserRemovesEmptyManualDNS(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "dns-orphan.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hash, _ := auth.HashPassword("pass12345!")
	u, _ := store.CreateVPNAccount(persist.User{
		Username: "solo", PasswordHash: hash, PublicKey: "pk", PrivateKeyEnc: "sk",
		VPNIP: "10.88.0.2", AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	id, err := store.CreateDNSServer("10.20.0.1", "only-u", []int64{u}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteUser(u); err != nil {
		t.Fatal(err)
	}
	d, err := store.GetDNSServer(id)
	if err != nil {
		t.Fatal(err)
	}
	if d != nil {
		t.Fatalf("空成员 manual DNS 应删除, got %+v", d)
	}
}

// TestDNSAppliesToUser 纯函数边界。
func TestDNSAppliesToUser(t *testing.T) {
	if !persist.DNSAppliesToUser([]int64{0}, nil, 3) {
		t.Fatal("all should hit")
	}
	if persist.DNSAppliesToUser([]int64{0}, []int64{3}, 3) {
		t.Fatal("excluded")
	}
	// 指定成员时排除忽略 → 仍生效
	if !persist.DNSAppliesToUser([]int64{2}, []int64{2}, 2) {
		t.Fatal("specific should apply even if listed in excludes")
	}
	if persist.DNSAppliesToUser([]int64{2}, nil, 3) {
		t.Fatal("other user miss")
	}
}

// TestCreateDNSServerRejectsEmptyMembers 空包含集须拒绝，不得静默变全部。
func TestCreateDNSServerRejectsEmptyMembers(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "dns-empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.CreateDNSServer("1.1.1.1", "", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "包含集不能为空") {
		t.Fatalf("want 包含集不能为空, got %v", err)
	}
	id, err := store.CreateDNSServer("1.1.1.1", "", []int64{persist.DNSMemberAll}, nil)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := store.GetDNSServer(id)
	if d == nil || !persist.PeerRouteHasAllMembers(d.MemberUserIDs) {
		t.Fatalf("explicit all should work: %+v", d)
	}
}

// TestCreateDNSServerUniqueChinese DNS IP 冲突须中文错误。
func TestCreateDNSServerUniqueChinese(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "dns-uniq.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hash, _ := auth.HashPassword("pass12345!")
	u, _ := store.CreateVPNAccount(persist.User{
		Username: "u", PasswordHash: hash, PublicKey: "pk", PrivateKeyEnc: "sk",
		VPNIP: "10.88.0.2", AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if _, err := store.CreateDNSServer("8.8.8.8", "", []int64{u}, nil); err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateDNSServer("8.8.8.8", "", []int64{u}, nil)
	if err == nil || !strings.Contains(err.Error(), "相同 DNS IP 已存在") {
		t.Fatalf("want 相同 DNS IP 已存在, got %v", err)
	}
}
