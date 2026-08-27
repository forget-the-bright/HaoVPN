package persist

import (
	"path/filepath"
	"testing"
	"time"

	"haovpn/internal/readmodel"
)

// TestListUsersPageFilter 分页与用户名筛选。
func TestListUsersPageFilter(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "page.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	hash := "dummy"
	id1, err := s.CreateVPNAccount(User{Username: "alice", PasswordHash: hash, PublicKey: "pk1", PrivateKeyEnc: "sk1", VPNIP: "10.0.0.2", IPMode: IPModeFixed, AllowedIPs: []string{"10.0.0.0/8"}})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.CreateVPNAccount(User{Username: "bob", PasswordHash: hash, PublicKey: "pk2", PrivateKeyEnc: "sk2", VPNIP: "10.0.0.3", IPMode: IPModeFixed, AllowedIPs: []string{"10.0.0.0/8"}})
	if err != nil {
		t.Fatal(err)
	}
	if id1 == 0 || id2 == 0 {
		t.Fatal("create ids")
	}

	n, _ := s.CountUsers()
	if n != 2 {
		t.Fatalf("count users=%d", n)
	}

	itemsAll, totalAll, err := s.ListUsersPage(readmodel.UserListFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if totalAll != 2 || len(itemsAll) != 2 {
		t.Fatalf("all total=%d len=%d items=%+v", totalAll, len(itemsAll), itemsAll)
	}

	items, total, err := s.ListUsersPage(readmodel.UserListFilter{Q: "alice", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].Username != "alice" {
		t.Fatalf("total=%d items=%+v", total, items)
	}
}

// TestPruneAuditAndEvents 保留策略删除旧记录。
func TestPruneAuditAndEvents(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "prune.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	hash := "dummy"
	_ = s.InsertAuditLog(AuditEntry{Action: "test", TargetType: "system"})
	uid, _ := s.CreateAdminUser("evtuser", hash, false)
	_ = s.InsertConnectionEvent(uid, "connect", "1.2.3.4", `{}`)
	_, _ = s.db.Exec(`UPDATE audit_logs SET created_at='2020-01-01 00:00:00'`)
	_, _ = s.db.Exec(`UPDATE connection_events SET created_at='2020-01-01 00:00:00'`)

	n1, _ := s.PruneAuditLogs(time.Now().AddDate(0, 0, -30))
	n2, _ := s.PruneConnectionEvents(time.Now().AddDate(0, 0, -30))
	if n1 < 1 || n2 < 1 {
		t.Fatalf("prune audit=%d events=%d", n1, n2)
	}
}
