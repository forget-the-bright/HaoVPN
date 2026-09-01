//go:build !windows

package winnet

import "fmt"

// CollectEgressSnapshot 非 Windows 不支持（ICS 出站仅 Windows）。
func CollectEgressSnapshot() (*EgressSnapshot, error) {
	return nil, fmt.Errorf("CollectEgressSnapshot: 仅 Windows 支持")
}
