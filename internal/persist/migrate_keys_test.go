package persist_test

import (
	"path/filepath"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/persist"
	"haovpn/internal/security"
)

// TestMigratePlaintextPrivateKeys 启动迁移须把明文私钥改成 enc:v1:。
func TestMigratePlaintextPrivateKeys(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mig.db")
	store, err := persist.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HashPassword("testpass12")
	id, err := store.CreateVPNAccount(persist.User{
		Username: "u", PasswordHash: hash, PublicKey: "pub-mig",
		PrivateKeyEnc: "legacy-plain-private-key", VPNIP: "10.88.0.9",
		AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	key := make([]byte, 32)
	enc, err := security.NewKeyEnc(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MigratePlaintextPrivateKeys(enc); err != nil {
		t.Fatal(err)
	}
	u, err := store.GetUserByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if !security.IsEncryptedPrivateKey(u.PrivateKeyEnc) {
		t.Fatalf("not migrated: %q", u.PrivateKeyEnc)
	}
	plain, err := enc.OpenPrivateKey(u.PrivateKeyEnc)
	if err != nil || plain != "legacy-plain-private-key" {
		t.Fatalf("open after migrate: %v %q", err, plain)
	}
	store.Close()
}
