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
	tooLong := strings.Repeat("a", 71) + "1" // 72 ok
	if err := auth.ValidatePasswordStrength(tooLong); err != nil {
		t.Fatalf("72 位合法密码被拒: %v", err)
	}
	if err := auth.ValidatePasswordStrength(tooLong + "x"); err == nil {
		t.Fatal("预期拒绝超过 72 位密码")
	}
}

func TestHashPasswordStrength(t *testing.T) {
	_, err := auth.HashPassword("weak")
	if err == nil || !strings.Contains(err.Error(), "8") {
		t.Fatalf("expected length error: %v", err)
	}
}
