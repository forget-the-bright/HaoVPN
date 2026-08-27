package security_test

import (
	"testing"

	"haovpn/internal/security"
)

func TestKeyEncRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	enc, err := security.NewKeyEnc(key)
	if err != nil {
		t.Fatal(err)
	}
	plain := "dGVzdC1wcml2YXRlLWtleS1iYXNlNjQ="
	sealed, err := enc.SealPrivateKey(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !security.IsEncryptedPrivateKey(sealed) {
		t.Fatal("expected enc:v1: prefix")
	}
	out, err := enc.OpenPrivateKey(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if out != plain {
		t.Fatalf("got %q want %q", out, plain)
	}
}

func TestOpenPlaintextMigration(t *testing.T) {
	key := make([]byte, 32)
	enc, _ := security.NewKeyEnc(key)
	plain := "legacy-plain-key"
	out, err := enc.OpenPrivateKey(plain)
	if err != nil || out != plain {
		t.Fatalf("plaintext passthrough: %v %q", err, out)
	}
}
