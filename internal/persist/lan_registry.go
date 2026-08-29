package persist

import (
	"fmt"
	"net"
	"strings"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/timeutil"
)

// ClientLANRegistry 客户端上报的本地网段临时广告一行。
type ClientLANRegistry struct {
	UserID    int64     `json:"user_id"`
	DestCIDR  string    `json:"dest_cidr"`
	VPNIP     string    `json:"vpn_ip"`
	HostID    string    `json:"host_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NormalizeLANCIDR 规范化本地网段 CIDR（禁止默认路由）。
func NormalizeLANCIDR(cidr string) (string, error) {
	return NormalizePeerRouteDest(cidr)
}

// ReplaceClientLANRegistry 用本次上报列表整表替换该账号注册行（换机覆盖）。
//
// cidrs 空：仅清空（等价于未配 local_lans 的客户端不应调用；服务端也可用于断线清理）。
func (s *Store) ReplaceClientLANRegistry(userID int64, vpnIP, hostID string, cidrs []string) error {
	if userID <= 0 {
		return fmt.Errorf("user_id 无效")
	}
	var normalized []string
	seen := map[string]struct{}{}
	for _, c := range cidrs {
		n, err := NormalizeLANCIDR(c)
		if err != nil {
			logger.Warn("lan_registry 跳过无效 CIDR user_id=%d cidr=%q: %v", userID, c, err)
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		normalized = append(normalized, n)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM client_lan_registry WHERE user_id=?`, userID); err != nil {
		return err
	}
	for _, dest := range normalized {
		if _, err := tx.Exec(
			`INSERT INTO client_lan_registry(user_id, dest_cidr, vpn_ip, host_id, updated_at)
			 VALUES(?,?,?,?,datetime('now'))`,
			userID, dest, strings.TrimSpace(vpnIP), strings.TrimSpace(hostID),
		); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	logger.Info("lan_registry_replaced user_id=%d vpn_ip=%s host_id=%s count=%d cidrs=%v",
		userID, vpnIP, hostID, len(normalized), normalized)
	return nil
}

// ClearClientLANRegistry 断线/踢线时清空该账号全部本地网段注册。
func (s *Store) ClearClientLANRegistry(userID int64) error {
	if userID <= 0 {
		return nil
	}
	res, err := s.db.Exec(`DELETE FROM client_lan_registry WHERE user_id=?`, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		logger.Info("lan_registry_cleared user_id=%d rows=%d", userID, n)
	}
	return nil
}

// ListClientLANRegistry 列出全部或按 via 过滤的注册行（控制台）。
func (s *Store) ListClientLANRegistry(viaUserID int64) ([]ClientLANRegistry, error) {
	var (
		q    string
		args []any
	)
	if viaUserID > 0 {
		q = `SELECT user_id, dest_cidr, vpn_ip, host_id, updated_at FROM client_lan_registry
			 WHERE user_id=? ORDER BY dest_cidr`
		args = []any{viaUserID}
	} else {
		q = `SELECT user_id, dest_cidr, vpn_ip, host_id, updated_at FROM client_lan_registry
			 ORDER BY user_id, dest_cidr`
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClientLANRegistry
	for rows.Next() {
		var r ClientLANRegistry
		var updated string
		if err := rows.Scan(&r.UserID, &r.DestCIDR, &r.VPNIP, &r.HostID, &updated); err != nil {
			return nil, err
		}
		r.UpdatedAt = timeutil.ParseUTC(updated)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ValidLANCIDRs 过滤并规范化 CIDR 列表（供客户端上报前校验）。
func ValidLANCIDRs(cidrs []string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, c := range cidrs {
		n, err := NormalizeLANCIDR(c)
		if err != nil {
			continue
		}
		// 额外拒绝明显非局域网用途的过宽前缀可选：仅禁 /0
		if _, ipnet, err := net.ParseCIDR(n); err == nil {
			ones, bits := ipnet.Mask.Size()
			if bits == 32 && ones == 0 {
				continue
			}
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}
