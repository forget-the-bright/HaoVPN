package tunnel

import (
	"encoding/json"
	"fmt"
)

// LANRegistryUpdate 客户端在确定 via 出口能力后纠正注册表（Handshake 帧，type=lan_registry）。
//
// 典型场景：Windows ICS 仅一块出站网卡，异网卡网段不可作 via → 握手后同步为 Active 列表。
type LANRegistryUpdate struct {
	Type      string   `json:"type"` // 固定 "lan_registry"
	LocalLANs []string `json:"local_lans"`
	HostID    string   `json:"host_id,omitempty"`
}

// EncodeLANRegistryUpdate 序列化注册表纠正帧。
func EncodeLANRegistryUpdate(localLANs []string, hostID string) ([]byte, error) {
	msg := LANRegistryUpdate{
		Type:      "lan_registry",
		LocalLANs: append([]string{}, localLANs...),
		HostID:    hostID,
	}
	return json.Marshal(msg)
}

// ParseLANRegistryUpdate 解析注册表纠正帧；type 必须为 lan_registry。
func ParseLANRegistryUpdate(data []byte) (LANRegistryUpdate, error) {
	var msg LANRegistryUpdate
	if err := json.Unmarshal(data, &msg); err != nil {
		return msg, fmt.Errorf("解析 lan_registry: %w", err)
	}
	if msg.Type != "lan_registry" {
		return msg, fmt.Errorf("无效 lan_registry 类型 %q", msg.Type)
	}
	return msg, nil
}
