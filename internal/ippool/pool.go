package ippool

import (
	"fmt"
	"net"
	"sync"

	"haovpn/internal/netutil"
)

// Pool 在 VPN 子网 CIDR 内为 peer 分配虚拟 IP 地址。
//
// 字段：
//   network — 解析后的 *net.IPNet，界定可分配范围。
//   reserved — 永久保留 IP（如服务端网关），Allocate 跳过。
//   allocated — ip 字符串 → peerID；peerID=0 表示临时占位。
//   next — 线性扫描游标；溢出后循环扫描至多 256 步。
//
// 线程安全：所有导出方法持 mu；可并发 Allocate/Release。
type Pool struct {
	mu        sync.Mutex
	network   *net.IPNet
	reserved  map[string]bool
	allocated map[string]int64 // ip -> peerID
	next      net.IP
}

// New 从 CIDR 字符串创建 IP 池，游标从网络地址 +1 起。
//
// 参数：cidr 须为合法 IPv4/IPv6 CIDR（如 10.88.0.0/24）。
// 返回：*Pool 与 err；ParseCIDR 失败时 Pool 为 nil。
// 副作用：跳过网络号作为首个候选地址。
func New(cidr string) (*Pool, error) {
	_, n, err := netutil.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("parse subnet: %w", err)
	}
	start := make(net.IP, len(n.IP))
	copy(start, n.IP)
	// Skip network address
	incIP(start)
	return &Pool{
		network:   n,
		reserved:  map[string]bool{},
		allocated: map[string]int64{},
		next:      start,
	}, nil
}

// Reserve 将指定 IP 标记为永久保留，不再分配给 peer。
//
// 参数：ip 为点分字符串；不必预先在子网内校验（Allocate 时仍会跳过）。
// 返回：无。
// 副作用：写入 reserved map；不释放已 allocated 的同 IP。
func (p *Pool) Reserve(ip string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reserved[ip] = true
}

// Allocate 为 peerID 分配下一个可用 IP（线性探测）。
//
// 参数：peerID 为会话/用户标识，写入 allocated 表。
// 返回：分配到的 IP 字符串；256 步内无空闲时 err 为「no free IPs」。
// 副作用：更新 next 游标与 allocated；持锁阻塞其他池操作。
func (p *Pool) Allocate(peerID int64) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := 0; i < 256; i++ {
		ip := p.next.String()
		if !p.reserved[ip] && p.allocated[ip] == 0 && p.network.Contains(p.next) {
			p.allocated[ip] = peerID
			incIP(p.next)
			return ip, nil
		}
		incIP(p.next)
	}
	return "", fmt.Errorf("no free IPs in pool")
}

// AllocateSpecific 将指定 IP 绑定给 peerID（可覆盖 peerID=0 的临时占用）。
//
// 参数：ip 须在 pool 子网内且非 reserved；已被其他非零 peer 占用时拒绝。
// 返回：格式/范围/冲突错误；成功时 nil。
// 副作用：写入或覆盖 allocated[ip]。
func (p *Pool) AllocateSpecific(ip string, peerID int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	parsed := net.ParseIP(ip)
	if parsed == nil || !p.network.Contains(parsed) {
		return fmt.Errorf("IP %s not in pool", ip)
	}
	if p.reserved[ip] {
		return fmt.Errorf("IP %s already allocated", ip)
	}
	if owner, ok := p.allocated[ip]; ok && owner != 0 && owner != peerID {
		return fmt.Errorf("IP %s already allocated", ip)
	}
	p.allocated[ip] = peerID
	return nil
}

// Release 将 IP 归还池中，供后续 Allocate 复用。
//
// 参数：ip 未分配时 delete 为 no-op。
// 返回：无。
// 副作用：从 allocated 删除条目；不影响 reserved。
func (p *Pool) Release(ip string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.allocated, ip)
}

// IsAllocated 判断 IP 是否已被占用（含 peerID=0 临时占位）。
//
// 参数：ip 为字符串形式。
// 返回：在 allocated 中存在为 true；reserved 但未 allocated 为 false。
// 副作用：无；持锁只读。
func (p *Pool) IsAllocated(ip string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.allocated[ip]
	return ok
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] != 0 {
			break
		}
	}
}
