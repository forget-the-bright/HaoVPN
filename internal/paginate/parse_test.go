package paginate

import "testing"

func TestParseIntDefault(t *testing.T) {
	if got := ParseIntDefault("", 50); got != 50 {
		t.Fatalf("empty: got %d", got)
	}
	if got := ParseIntDefault("abc", 50); got != 50 {
		t.Fatalf("invalid: got %d", got)
	}
	if got := ParseIntDefault("100", 50); got != 100 {
		t.Fatalf("valid: got %d", got)
	}
}
