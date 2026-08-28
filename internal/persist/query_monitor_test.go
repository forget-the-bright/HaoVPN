package persist_test

import (
	"path/filepath"
	"testing"

	"haovpn/internal/persist"
	"haovpn/internal/readmodel"
)

func TestListMonitorAccountRowsNameFilter(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "mon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	id, err := store.CreateVPNAccount(persist.User{
		Username: "alice", PasswordHash: "h", PublicKey: "pk1", VPNIP: "10.88.0.2", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateVPNAccount(persist.User{
		Username: "bob", PasswordHash: "h", PublicKey: "pk2", VPNIP: "10.88.0.3", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	rows, err := store.ListMonitorAccountRows(readmodel.MonitorAccountFilter{NameQuery: "ali"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != id || rows[0].Username != "alice" {
		t.Fatalf("filter ali: got %+v", rows)
	}
}

func TestListConnectionEventsFilteredJoinUsername(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "evt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	u, err := store.CreateVPNAccount(persist.User{
		Username: "evtuser", PasswordHash: "h", PublicKey: "pk", VPNIP: "10.88.0.4", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.InsertConnectionEvent(u, "connect", "1.2.3.4:1234", ""); err != nil {
		t.Fatal(err)
	}
	rows, total, err := store.ListConnectionEventsFiltered(readmodel.ConnectionEventFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("total=%d len=%d", total, len(rows))
	}
	if rows[0].Username != "evtuser" {
		t.Fatalf("username=%q", rows[0].Username)
	}
}
