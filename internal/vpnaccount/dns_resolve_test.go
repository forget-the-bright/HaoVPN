package vpnaccount

import (
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/persist"
)

// TestResolveDNSForUserExclude 覆盖 all−exclude 与 gateway 回落。
func TestResolveDNSForUserExclude(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(dir + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	hash, _ := auth.HashPassword("pass12345!")
	home, err := store.CreateVPNAccount(persist.User{
		Username: "home", PasswordHash: hash, PublicKey: "pk1", PrivateKeyEnc: "sk",
		VPNIP: "10.88.0.2", AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	corp, err := store.CreateVPNAccount(persist.User{
		Username: "corp", PasswordHash: hash, PublicKey: "pk2", PrivateKeyEnc: "sk",
		VPNIP: "10.88.0.3", AllowedIPs: []string{"10.88.0.0/24", "10.10.0.0/16"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.SyncConfigDNSServers([]string{"10.10.6.51"}); err != nil {
		t.Fatal(err)
	}
	list, _ := store.ListDNSServers()
	if len(list) != 1 {
		t.Fatal(list)
	}
	if err := store.ReplaceDNSServerExcludes(list[0].ID, []int64{home}); err != nil {
		t.Fatal(err)
	}

	svc := &Service{
		Store: store,
		Cfg: &config.ServerConfig{VPN: config.VPNSection{GatewayIP: "10.88.0.1"}},
	}
	gotHome := svc.ResolveDNSForUser(home, "10.88.0.1")
	if len(gotHome) != 1 || gotHome[0] != "10.88.0.1" {
		t.Fatalf("home fallback want gateway got %v", gotHome)
	}
	gotCorp := svc.ResolveDNSForUser(corp, "10.88.0.1")
	if len(gotCorp) != 1 || gotCorp[0] != "10.10.6.51" {
		t.Fatalf("corp want 10.10.6.51 got %v", gotCorp)
	}
}
