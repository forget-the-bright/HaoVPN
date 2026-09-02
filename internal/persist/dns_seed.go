package persist

import (
	"fmt"
	"strings"

	"haovpn/internal/logger"
)

// dns_seed.go：从 server.yaml vpn.dns_servers 同步 source=config 行。

const defaultConfigDNSRemark = "来自 server.yaml"

// SyncConfigDNSServers 将 YAML DNS 列表同步为 source=config 行。
//
// 规则（产品锁定）：
//   - YAML 中有的 IP：保证存在 source=config、members=[0]、备注缺省补默认文案；不改已有 excludes / 已改备注。
//   - YAML 中无的 config 行：删除（CASCADE 清 members/excludes）。
//   - manual 行不受影响；若 manual 与 YAML IP 冲突，跳过该 YAML IP 并打 WARN（保留手工行）。
//
// 返回：新增/更新/删除计数（排障日志用）。
func (s *Store) SyncConfigDNSServers(yamlDNS []string) (added, kept, removed int, err error) {
	want := map[string]struct{}{}
	var ordered []string
	for _, raw := range yamlDNS {
		ip, vErr := ValidateDNSIP(raw)
		if vErr != nil {
			logger.Warn("dns_seed 跳过无效 YAML dns: %v", vErr)
			continue
		}
		if _, ok := want[ip]; ok {
			continue
		}
		want[ip] = struct{}{}
		ordered = append(ordered, ip)
	}

	existing, err := s.ListDNSServers()
	if err != nil {
		return 0, 0, 0, err
	}
	configByIP := map[string]DNSServer{}
	manualIPs := map[string]struct{}{}
	for _, d := range existing {
		if d.IsConfigSource() {
			configByIP[d.DNSIP] = d
		} else {
			manualIPs[d.DNSIP] = struct{}{}
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, 0, err
	}
	defer tx.Rollback()

	for _, ip := range ordered {
		if _, clash := manualIPs[ip]; clash {
			logger.Warn("dns_seed YAML dns_ip=%s 与手工托管 DNS 冲突，保留手工行、跳过 seed", ip)
			continue
		}
		if d, ok := configByIP[ip]; ok {
			// 纠正 members 必须为 all
			if _, err := tx.Exec(`DELETE FROM dns_server_members WHERE dns_id=?`, d.ID); err != nil {
				return 0, 0, 0, err
			}
			if _, err := tx.Exec(`INSERT INTO dns_server_members(dns_id, user_id) VALUES(?,?)`, d.ID, DNSMemberAll); err != nil {
				return 0, 0, 0, err
			}
			if strings.TrimSpace(d.Remark) == "" {
				if _, err := tx.Exec(`UPDATE dns_servers SET remark=?, updated_at=datetime('now') WHERE id=?`,
					defaultConfigDNSRemark, d.ID); err != nil {
					return 0, 0, 0, err
				}
			}
			kept++
			delete(configByIP, ip)
			continue
		}
		res, err := tx.Exec(
			`INSERT INTO dns_servers(dns_ip, remark, source) VALUES(?,?,?)`,
			ip, defaultConfigDNSRemark, DNSSourceConfig,
		)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("dns_seed insert: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return 0, 0, 0, err
		}
		if _, err := tx.Exec(`INSERT INTO dns_server_members(dns_id, user_id) VALUES(?,?)`, id, DNSMemberAll); err != nil {
			return 0, 0, 0, err
		}
		added++
	}

	for _, stale := range configByIP {
		if _, err := tx.Exec(`DELETE FROM dns_server_members WHERE dns_id=?`, stale.ID); err != nil {
			return 0, 0, 0, err
		}
		if _, err := tx.Exec(`DELETE FROM dns_server_excludes WHERE dns_id=?`, stale.ID); err != nil {
			return 0, 0, 0, err
		}
		if _, err := tx.Exec(`DELETE FROM dns_servers WHERE id=?`, stale.ID); err != nil {
			return 0, 0, 0, err
		}
		removed++
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, 0, err
	}
	return added, kept, removed, nil
}
