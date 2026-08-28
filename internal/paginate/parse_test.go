package paginate

import (
	"net/url"
	"testing"
)

func TestParseBoolQuery(t *testing.T) {
	cases := []struct {
		in      string
		val, ok bool
	}{
		{"1", true, true},
		{"true", true, true},
		{"0", false, true},
		{"false", false, true},
		{"", false, false},
		{"yes", false, false},
	}
	for _, c := range cases {
		val, ok := ParseBoolQuery(c.in)
		if val != c.val || ok != c.ok {
			t.Fatalf("ParseBoolQuery(%q) = (%v,%v), want (%v,%v)", c.in, val, ok, c.val, c.ok)
		}
	}
}

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

func TestParseLimitOffset(t *testing.T) {
	q := url.Values{}
	limit, offset := ParseLimitOffset(q, 50, 500)
	if limit != 50 || offset != 0 {
		t.Fatalf("empty: limit=%d offset=%d", limit, offset)
	}
	q.Set("limit", "100")
	q.Set("offset", "20")
	limit, offset = ParseLimitOffset(q, 50, 500)
	if limit != 100 || offset != 20 {
		t.Fatalf("valid: limit=%d offset=%d", limit, offset)
	}
	q.Set("limit", "9999")
	limit, offset = ParseLimitOffset(q, 50, 500)
	if limit != 500 {
		t.Fatalf("clamp max: limit=%d", limit)
	}
	q.Set("limit", "abc")
	q.Set("offset", "x")
	limit, offset = ParseLimitOffset(q, 50, 200)
	if limit != 50 || offset != 0 {
		t.Fatalf("invalid: limit=%d offset=%d", limit, offset)
	}
}
