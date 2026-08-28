package timeutil

import (
	"testing"
	"time"
)

func TestFormatRFC3339(t *testing.T) {
	tm := time.Date(2026, 8, 28, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	got := FormatRFC3339(tm)
	if got != "2026-08-28T04:00:00Z" {
		t.Fatalf("FormatRFC3339: got %q", got)
	}
	if FormatRFC3339Ptr(nil) != "" {
		t.Fatal("nil ptr should be empty")
	}
}

func TestParseSinceRFC3339(t *testing.T) {
	if !ParseSinceRFC3339("").IsZero() {
		t.Fatal("empty should be zero")
	}
	if !ParseSinceRFC3339("not-a-time").IsZero() {
		t.Fatal("invalid should be zero")
	}
	got := ParseSinceRFC3339("2026-08-28T04:00:00Z")
	if got.UTC().Format(time.RFC3339) != "2026-08-28T04:00:00Z" {
		t.Fatalf("got %v", got)
	}
}
