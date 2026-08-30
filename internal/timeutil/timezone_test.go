package timeutil_test

import (
	"testing"
	"time"

	"haovpn/internal/timeutil"
)

func TestLoadDisplayLocation(t *testing.T) {
	cases := []struct {
		in      string
		wantOff int // seconds east of UTC；IANA 用抽样时刻
		iana    bool
	}{
		{"", 0, false},
		{"UTC", 0, false},
		{"GMT+8", 8 * 3600, false},
		{"UTC+8", 8 * 3600, false},
		{"+08:00", 8 * 3600, false},
		{"+8", 8 * 3600, false},
		{"-05:00", -5 * 3600, false},
		{"Asia/Shanghai", 8 * 3600, true},
	}
	at := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	for _, tc := range cases {
		loc, err := timeutil.LoadDisplayLocation(tc.in)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		_, off := at.In(loc).Zone()
		if off != tc.wantOff {
			t.Fatalf("%q offset=%d want %d", tc.in, off, tc.wantOff)
		}
	}
	if _, err := timeutil.LoadDisplayLocation("Not/AZone"); err == nil {
		t.Fatal("非法时区应失败")
	}
}

func TestFormatInDisplayGMT8(t *testing.T) {
	loc, err := timeutil.LoadDisplayLocation("GMT+8")
	if err != nil {
		t.Fatal(err)
	}
	utc := time.Date(2026, 8, 30, 2, 0, 0, 0, time.UTC)
	got := timeutil.FormatInDisplay(utc, loc)
	if got != "2026-08-30 10:00:00 +08:00" {
		t.Fatalf("got %q", got)
	}
}
