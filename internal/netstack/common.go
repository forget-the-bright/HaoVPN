package netstack

// WFPFilterRef 枚举到的 WFP 过滤器摘要（纯数据，供单测与崩溃残留清理逻辑使用）。
//
// 字段：
//   ID — FwpmFilterEnum0 返回的 filterId。
//   Sublayer — 子层 GUID 的 16 字节原始形式，与 HaoVPNKillSublayerBytes 比对。
type WFPFilterRef struct {
	ID       uint64
	Sublayer [16]byte // GUID 原始字节
}

// SelectProductFilterIDs 从枚举结果中筛出属于 HaoVPN 杀开关子层的过滤器 ID。
//
// 参数：
//   items — enumLayerFiltersLocked 或测试注入的枚举列表。
//   productSublayer — 本产品固定子层 GUID 字节（HaoVPNKillSublayerBytes）。
// 返回：待 FwpmFilterDeleteById0 删除的 ID 列表。
func SelectProductFilterIDs(items []WFPFilterRef, productSublayer [16]byte) []uint64 {
	var out []uint64
	for _, it := range items {
		if it.Sublayer == productSublayer {
			out = append(out, it.ID)
		}
	}
	return out
}

// HaoVPNKillSublayerBytes 返回本产品 WFP 杀开关子层 GUID 的固定 16 字节形式。
//
// 返回：与 killswitch_windows 中 HaoVPNSublayerGUID 一致（{a1b2c3d4-e5f6-7890-abcd-ef0123456789} 小端编码）。
// 用途：跨进程/崩溃后按子层 GUID 清理残留 Block 过滤器。
func HaoVPNKillSublayerBytes() [16]byte {
	// {a1b2c3d4-e5f6-7890-abcd-ef0123456789} 小端 Data1/Data2/Data3 + Data4
	return [16]byte{
		0xd4, 0xc3, 0xb2, 0xa1, 0xf6, 0xe5, 0x90, 0x78,
		0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67, 0x89,
	}
}
