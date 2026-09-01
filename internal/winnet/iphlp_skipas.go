package winnet

import "haovpn/internal/netutil"

// preferSkipAsSourceNeedsUpdate 委托 netutil（包内 iphlp 用）。
func preferSkipAsSourceNeedsUpdate(vpnSkip, has137, skip137 bool) bool {
	return netutil.PreferSkipAsSourceNeedsUpdate(vpnSkip, has137, skip137)
}
