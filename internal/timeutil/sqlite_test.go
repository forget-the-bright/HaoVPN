package timeutil

import (
	"database/sql"
	"testing"
	"time"
)

func TestFormatParseUTC_RoundTrip(t *testing.T) {
	in := time.Date(2026, 8, 28, 12, 34, 56, 0, time.UTC)
	s := FormatUTC(in)
	if s != "2026-08-28 12:34:56" {
		t.Fatalf("FormatUTC=%q", s)
	}
	out := ParseUTC(s)
	if !out.Equal(in) {
		t.Fatalf("ParseUTC=%v want %v", out, in)
	}
}

func TestFormatUTC_ConvertsLocal(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	in := time.Date(2026, 8, 28, 20, 0, 0, 0, loc)
	s := FormatUTC(in)
	if s != "2026-08-28 12:00:00" {
		t.Fatalf("got %q", s)
	}
}

func TestParseUTC_Invalid(t *testing.T) {
	if !ParseUTC("not-a-time").IsZero() {
		t.Fatal("expected zero")
	}
	if !ParseUTC("").IsZero() {
		t.Fatal("expected zero")
	}
}

func TestParseUTCPtr(t *testing.T) {
	if ParseUTCPtr(sql.NullString{}) != nil {
		t.Fatal("invalid null")
	}
	if ParseUTCPtr(sql.NullString{Valid: true, String: ""}) != nil {
		t.Fatal("empty")
	}
	p := ParseUTCPtr(sql.NullString{Valid: true, String: "2026-01-02 03:04:05"})
	if p == nil || p.Year() != 2026 {
		t.Fatalf("got %#v", p)
	}
}
