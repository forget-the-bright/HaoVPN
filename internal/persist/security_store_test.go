package persist_test

import (
	"testing"
	"time"

	"haovpn/internal/persist"
)

// TestSecurityEventsAndIPBlocks 验证探针事件与封禁 CRUD。
func TestSecurityEventsAndIPBlocks(t *testing.T) {
	store, err := persist.Open(t.TempDir() + "/sec.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.InsertSecurityEvent(persist.SecurityEvent{
		ClientIP: "1.2.3.4", ClientPort: "12345",
		Phase: "frame", Signature: "http_get", Action: "rejected",
		DetailJSON: `{"len":1195725856}`,
	}); err != nil {
		t.Fatal(err)
	}
	list, total, err := store.ListSecurityEvents(persist.SecurityEventFilter{Limit: 10})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("list events total=%d len=%d err=%v", total, len(list), err)
	}
	n, err := store.CountSecurityEventsSince("1.2.3.4", time.Now().Add(-time.Hour), nil)
	if err != nil || n != 1 {
		t.Fatalf("count=%d err=%v", n, err)
	}

	exp := time.Now().Add(time.Hour)
	if err := store.UpsertIPBlock(persist.IPBlock{
		IP: "1.2.3.4", Reason: "auto test", Source: "auto",
		Signature: "http_get", Enabled: true, ExpiresAt: &exp,
	}); err != nil {
		t.Fatal(err)
	}
	b, err := store.GetActiveIPBlock("1.2.3.4")
	if err != nil || b == nil {
		t.Fatalf("active block: %v %v", b, err)
	}
	if err := store.IncrementIPBlockHit("1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	if err := store.DisableIPBlock("1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	b2, err := store.GetActiveIPBlock("1.2.3.4")
	if err != nil || b2 != nil {
		t.Fatalf("after disable expect nil, got %v err=%v", b2, err)
	}
}
