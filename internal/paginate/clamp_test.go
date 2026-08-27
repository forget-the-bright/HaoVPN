package paginate

import "testing"

func TestClampLimit(t *testing.T) {
	tests := []struct {
		limit, def, max, want int
	}{
		{0, 50, 500, 50},
		{-1, 50, 500, 50},
		{100, 50, 500, 100},
		{999, 50, 500, 500},
		{50, 50, 500, 50},
	}
	for _, tc := range tests {
		if got := ClampLimit(tc.limit, tc.def, tc.max); got != tc.want {
			t.Errorf("ClampLimit(%d,%d,%d)=%d want %d", tc.limit, tc.def, tc.max, got, tc.want)
		}
	}
}

func TestClampOffset(t *testing.T) {
	if ClampOffset(-5) != 0 {
		t.Fatal("negative offset")
	}
	if ClampOffset(10) != 10 {
		t.Fatal("positive offset")
	}
}
