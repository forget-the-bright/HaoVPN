package paginate

// ClampLimit 将分页 limit 限制在 [defaultVal, maxVal] 范围内。
//
// 参数：
//   limit — 调用方传入的每页条数；≤0 时使用 defaultVal。
//   defaultVal — 缺省或非法时的默认值（须 >0）。
//   maxVal — 允许的最大条数（须 ≥ defaultVal）。
//
// 返回：规范化后的 limit，恒满足 defaultVal ≤ result ≤ maxVal（当 defaultVal ≤ maxVal 时）。
// 副作用：无。
func ClampLimit(limit, defaultVal, maxVal int) int {
	if limit <= 0 {
		return defaultVal
	}
	if limit > maxVal {
		return maxVal
	}
	return limit
}

// ClampOffset 将分页 offset 规范为非负值。
//
// 参数：offset — 跳过条数；<0 时视为 0。
// 返回：≥0 的 offset。
func ClampOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}
