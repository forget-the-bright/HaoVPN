package netutil

// ResolveMTU 按顺序取第一个大于 0 的 MTU 候选值。
//
// 参数：values — 通常为「握手 policy → 本地 tun.mtu → 0 占位」三级回退链。
// 返回：首个正值；全部为 0 时返回 DefaultMTU。
func ResolveMTU(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return DefaultMTU
}

// ReadBufferSize 返回 TUN 设备读循环应使用的缓冲长度（MTU + TunReadBufferExtra）。
//
// 参数：mtu — ≤0 时使用 DefaultMTU。
func ReadBufferSize(mtu int) int {
	if mtu <= 0 {
		mtu = DefaultMTU
	}
	return mtu + TunReadBufferExtra
}
