package winnet

import "testing"

func TestIsHomeSKUStrings(t *testing.T) {
	cases := []struct {
		edition, product string
		want             bool
	}{
		{"Core", "Windows 10 Home", true},
		{"CoreSingleLanguage", "Windows 11 Home", true},
		{"CoreN", "Windows 10 Home N", true},
		{"CoreCountrySpecific", "Windows 10 Home China", true},
		{"Professional", "Windows 10 Pro", false},
		{"Enterprise", "Windows 10 Enterprise", false},
		{"", "Windows 11 Home", true},
		{"Professional", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := IsHomeSKUStrings(c.edition, c.product); got != c.want {
			t.Fatalf("IsHomeSKUStrings(%q,%q)=%v want %v", c.edition, c.product, got, c.want)
		}
	}
}
