package auth_test

import (
	"strings"
	"testing"

	"haovpn/internal/auth"
)

func TestValidatePasswordStrength(t *testing.T) {
	if err := auth.ValidatePasswordStrength("short1"); err == nil {
		t.Fatal("expected too short")
	}
	if err := auth.ValidatePasswordStrength("allletters"); err == nil {
		t.Fatal("expected missing digit")
	}
	if err := auth.ValidatePasswordStrength("12345678"); err == nil {
		t.Fatal("expected missing letter")
	}
	if err := auth.ValidatePasswordStrength("Secure123"); err != nil {
		t.Fatalf("valid password rejected: %v", err)
	}
}

func TestHashPasswordStrength(t *testing.T) {
	_, err := auth.HashPassword("weak")
	if err == nil || !strings.Contains(err.Error(), "8") {
		t.Fatalf("expected length error: %v", err)
	}
}
