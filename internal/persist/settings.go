package persist

import (
	"database/sql"
	"fmt"

	"haovpn/internal/paginate"
)

const (
	// SettingAllowAllVPNPeers 运行时覆盖 YAML security.allow_all_vpn_peers（"1"/"0"）。
	SettingAllowAllVPNPeers = "allow_all_vpn_peers"
)

// GetSetting 读取 schema_meta 键；不存在返回 ("", nil)。
func (s *Store) GetSetting(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM schema_meta WHERE key=?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// SetSetting 写入或更新 schema_meta。
func (s *Store) SetSetting(key, value string) error {
	if key == "" {
		return fmt.Errorf("setting key 为空")
	}
	_, err := s.db.Exec(
		`INSERT INTO schema_meta(key, value) VALUES(?,?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		key, value,
	)
	return err
}

// GetAllowAllVPNPeersSetting 读取互访全局开关覆盖；ok=false 表示未配置（沿用 YAML）。
func (s *Store) GetAllowAllVPNPeersSetting() (allow bool, ok bool, err error) {
	v, err := s.GetSetting(SettingAllowAllVPNPeers)
	if err != nil || v == "" {
		return false, false, err
	}
	val, parsed := paginate.ParseBoolQuery(v)
	if !parsed {
		return false, false, nil
	}
	return val, true, nil
}

// SetAllowAllVPNPeersSetting 持久化互访全局开关。
func (s *Store) SetAllowAllVPNPeersSetting(allow bool) error {
	v := "0"
	if allow {
		v = "1"
	}
	return s.SetSetting(SettingAllowAllVPNPeers, v)
}
