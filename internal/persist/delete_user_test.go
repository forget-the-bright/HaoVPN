package persist_test

import (
	"path/filepath"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/persist"
)

// TestDeleteUserCascadesFK 有连接事件/IP 占用时删除账号不得 FK 787。
func TestDeleteUserCascadesFK(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "del.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	hash, _ := auth.HashPassword("pass12345!")
	id, err := store.CreateVPNAccount(persist.User{
		Username: "todel", PasswordHash: hash, PublicKey: "pk-del", PrivateKeyEnc: "sk",
		VPNIP: "10.88.0.99", AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordIPAllocation("10.88.0.99", id); err != nil {
		t.Fatal(err)
	}
	_, err = store.DB().Exec(`INSERT INTO connection_events(user_id, event_type, remote_addr) VALUES(?,?,?)`,
		id, "connect", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DB().Exec(`INSERT INTO session_stats(user_id, rx_bytes, tx_bytes) VALUES(?,?,?)`, id, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	aid := id
	if err := store.InsertAuditLog(persist.AuditEntry{ActorUserID: &aid, Action: "login", TargetType: "user"}); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteUser(id); err != nil {
		t.Fatalf("DeleteUser FK fail: %v", err)
	}
	if _, err := store.GetUserByID(id); err == nil {
		t.Fatal("user should be gone")
	}
	var n int
	_ = store.DB().QueryRow(`SELECT COUNT(*) FROM ip_allocations WHERE user_id=?`, id).Scan(&n)
	if n != 0 {
		t.Fatal("ip_allocations not cleaned")
	}
	_ = store.DB().QueryRow(`SELECT COUNT(*) FROM connection_events WHERE user_id=?`, id).Scan(&n)
	if n != 0 {
		t.Fatal("connection_events not cleaned")
	}
	_ = store.DB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE actor_user_id=?`, id).Scan(&n)
	if n != 0 {
		t.Fatal("audit actor should be nulled")
	}
}
