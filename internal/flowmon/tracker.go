// Package flowmon 提供服务端 L3/L4 流量流表（内存聚合，无 DPI/L7）。
//
// 热路径：sessionmgr HandleInbound / sendToAccount 调用 Observe*；禁止每包写库。
// 上游：api/monitor flows、连接页「流量明细」。
// 下游：无（纯内存）；可选 TTL 淘汰与每用户上限。
package flowmon

import (
	"encoding/binary"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// DirIn 客户端→远端（隧道入站明文）。
// DirOut 远端→客户端（发往账号的出站明文）。
const (
	DirIn  = 1
	DirOut = 2
)

// observeTTLEvery：同一分片每累计这么多次 Observe 才扫一次 TTL，避免每包全表扫描。
const observeTTLEvery = 256

// Proto 名称便于 UI。
func ProtoName(p uint8) string {
	switch p {
	case 1:
		return "ICMP"
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	default:
		return "IP"
	}
}

// Key 规范化五元组：Src* 为客户端侧，Dst* 为远端侧（双向合并）。
type Key struct {
	UserID int64
	SrcIP  string
	DstIP  string
	Proto  uint8
	Sport  uint16
	Dport  uint16
}

// Flow 聚合计数。
type Flow struct {
	Key
	BytesIn    int64
	BytesOut   int64
	PacketsIn  int64
	PacketsOut int64
	FirstSeen  time.Time
	LastSeen   time.Time
}

// Snapshot 只读副本供 API。
type Snapshot struct {
	UserID     int64     `json:"user_id"`
	SrcIP      string    `json:"src_ip"`
	DstIP      string    `json:"dst_ip"`
	Proto      uint8     `json:"proto"`
	ProtoName  string    `json:"proto_name"`
	Sport      uint16    `json:"sport"`
	Dport      uint16    `json:"dport"`
	BytesIn    int64     `json:"bytes_in"`
	BytesOut   int64     `json:"bytes_out"`
	PacketsIn  int64     `json:"packets_in"`
	PacketsOut int64     `json:"packets_out"`
	BytesTotal int64     `json:"bytes_total"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
}

// Tracker 分片内存流表。
type Tracker struct {
	shards          [16]flowShard
	ttl             time.Duration
	maxPerUser      int
	observeTTLEvery uint64
}

type flowShard struct {
	mu       sync.Mutex
	byKey    map[Key]*Flow
	observeN uint64 // 本分片 Observe 计数，用于低频 TTL
}

// Options 构造参数。
type Options struct {
	TTL time.Duration // 无活动淘汰；默认 10m
	// MaxPerUser 每用户最多流条数（跨全部分片合计）；≤0 表示默认 512。
	MaxPerUser int
	// ObserveTTLEvery 同一分片每累计多少次 Observe 扫一次 TTL；≤0 默认 256（单测可设 1）。
	ObserveTTLEvery int
}

// New 创建 Tracker。
func New(opt Options) *Tracker {
	if opt.TTL <= 0 {
		opt.TTL = 10 * time.Minute
	}
	if opt.MaxPerUser <= 0 {
		opt.MaxPerUser = 512
	}
	every := opt.ObserveTTLEvery
	if every <= 0 {
		every = observeTTLEvery
	}
	t := &Tracker{ttl: opt.TTL, maxPerUser: opt.MaxPerUser, observeTTLEvery: uint64(every)}
	for i := range t.shards {
		t.shards[i].byKey = make(map[Key]*Flow)
	}
	return t
}

func (t *Tracker) shardIndex(k Key) int {
	h := uint64(k.UserID)
	h ^= uint64(k.Proto) << 8
	h ^= uint64(k.Sport)<<16 ^ uint64(k.Dport)<<32
	for _, c := range k.DstIP {
		h = h*31 + uint64(c)
	}
	return int(h % uint64(len(t.shards)))
}

// ObservePacket 解析 IPv4 明文并按方向累加。
//
// dir=DirIn：包来自客户端，Src=客户端、Dst=远端。
// dir=DirOut：包发往客户端，线上 Src=远端、Dst=客户端 → 规范化交换后合并。
//
// 已存在流：只锁目标分片更新（热路径）。
// 新流：按分片下标 0→15 整表短锁，计数+淘汰+插入一体完成，避免并发 TOCTOU 突破 maxPerUser。
// 整表锁仅新 key，可接受（监控旁路，非转发关键路径）。
func (t *Tracker) ObservePacket(userID int64, packet []byte, dir int) {
	if t == nil || userID <= 0 || len(packet) < 20 {
		return
	}
	if packet[0]>>4 != 4 {
		return
	}
	ihl := int(packet[0]&0x0f) * 4
	if ihl < 20 || len(packet) < ihl {
		return
	}
	proto := packet[9]
	srcIP := net.IP(packet[12:16]).String()
	dstIP := net.IP(packet[16:20]).String()
	var sport, dport uint16
	if proto == 6 || proto == 17 {
		if len(packet) < ihl+4 {
			return
		}
		sport = binary.BigEndian.Uint16(packet[ihl : ihl+2])
		dport = binary.BigEndian.Uint16(packet[ihl+2 : ihl+4])
	}
	k := Key{UserID: userID, SrcIP: srcIP, DstIP: dstIP, Proto: proto, Sport: sport, Dport: dport}
	if dir == DirOut {
		k.SrcIP, k.DstIP = dstIP, srcIP
		k.Sport, k.Dport = dport, sport
	}
	n := int64(len(packet))
	now := time.Now()
	idx := t.shardIndex(k)
	sh := &t.shards[idx]

	// 热路径：已存在则单分片更新
	sh.mu.Lock()
	sh.observeN++
	if sh.observeN%t.observeTTLEvery == 0 {
		t.purgeShardTTLLocked(sh, now.Add(-t.ttl))
	}
	if f := sh.byKey[k]; f != nil {
		t.bumpFlowLocked(f, n, dir, now)
		sh.mu.Unlock()
		return
	}
	sh.mu.Unlock()

	// 新流：整表锁序保证 maxPerUser
	t.lockAllShards()
	defer t.unlockAllShards()
	sh = &t.shards[idx]
	if f := sh.byKey[k]; f != nil {
		t.bumpFlowLocked(f, n, dir, now)
		return
	}
	if t.maxPerUser > 0 {
		for t.countUserLocked(userID) >= t.maxPerUser {
			if !t.evictOldestUserLocked(userID) {
				break
			}
		}
	}
	f := &Flow{Key: k, FirstSeen: now}
	sh.byKey[k] = f
	t.bumpFlowLocked(f, n, dir, now)
}

func (t *Tracker) bumpFlowLocked(f *Flow, n int64, dir int, now time.Time) {
	f.LastSeen = now
	if dir == DirIn {
		f.BytesIn += n
		f.PacketsIn++
	} else {
		f.BytesOut += n
		f.PacketsOut++
	}
}

// lockAllShards / unlockAllShards：固定 0→15 / 15→0，避免多 goroutine 嵌套死锁。
func (t *Tracker) lockAllShards() {
	for i := range t.shards {
		t.shards[i].mu.Lock()
	}
}

func (t *Tracker) unlockAllShards() {
	for i := len(t.shards) - 1; i >= 0; i-- {
		t.shards[i].mu.Unlock()
	}
}

// purgeShardTTLLocked 淘汰本分片过期流（调用方须已持 sh.mu）。
func (t *Tracker) purgeShardTTLLocked(sh *flowShard, cutoff time.Time) {
	for k, fl := range sh.byKey {
		if fl.LastSeen.Before(cutoff) {
			delete(sh.byKey, k)
		}
	}
}

// countUserLocked 跨片统计（调用方须已持全部 shard.mu）。
func (t *Tracker) countUserLocked(userID int64) int {
	n := 0
	for i := range t.shards {
		for k := range t.shards[i].byKey {
			if k.UserID == userID {
				n++
			}
		}
	}
	return n
}

// evictOldestUserLocked 删该用户最旧一条（调用方须已持全部 shard.mu）。
func (t *Tracker) evictOldestUserLocked(userID int64) bool {
	var oldest Key
	var oldestT time.Time
	var oldestIdx int
	first := true
	for i := range t.shards {
		for k, f := range t.shards[i].byKey {
			if k.UserID != userID {
				continue
			}
			if first || f.LastSeen.Before(oldestT) {
				oldest, oldestT, oldestIdx, first = k, f.LastSeen, i, false
			}
		}
	}
	if first {
		return false
	}
	delete(t.shards[oldestIdx].byKey, oldest)
	return true
}

// ListFilter 列表过滤与排序。
type ListFilter struct {
	UserID int64
	Proto  int    // -1 或 0=全部；否则精确匹配
	SrcIP  string // 子串包含客户端 IP；空=不限
	DstIP  string // 子串包含远端 IP；空=不限
	Sort   []SortKey
	Limit  int
	Offset int
}

// SortKey 多列排序一项；Dir 为 asc / desc（其它按 desc）。
type SortKey struct {
	Key string // rx_bytes | tx_bytes | packets_in | packets_out | last_seen
	Dir string
}

// 合法排序键。
const (
	SortRXBytes    = "rx_bytes"
	SortTXBytes    = "tx_bytes"
	SortPacketsIn  = "packets_in"
	SortPacketsOut = "packets_out"
	SortLastSeen   = "last_seen"
)

// ParseSortQuery 解析 "rx_bytes:desc,last_seen:asc"；非法键跳过。
func ParseSortQuery(raw string) []SortKey {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []SortKey
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, dir, ok := strings.Cut(part, ":")
		if !ok {
			key, dir = part, "desc"
		}
		key = strings.TrimSpace(strings.ToLower(key))
		dir = strings.TrimSpace(strings.ToLower(dir))
		if !validSortKey(key) {
			continue
		}
		if dir != "asc" && dir != "desc" {
			dir = "desc"
		}
		out = append(out, SortKey{Key: key, Dir: dir})
	}
	return out
}

func validSortKey(k string) bool {
	switch k {
	case SortRXBytes, SortTXBytes, SortPacketsIn, SortPacketsOut, SortLastSeen:
		return true
	default:
		return false
	}
}

// List 过滤 → 多键排序 → 分页；顺带淘汰过期流。缺省排序 last_seen desc。
func (t *Tracker) List(f ListFilter) (items []Snapshot, total int) {
	if t == nil {
		return nil, 0
	}
	now := time.Now()
	cutoff := now.Add(-t.ttl)
	srcQ := strings.TrimSpace(f.SrcIP)
	dstQ := strings.TrimSpace(f.DstIP)
	var all []Snapshot
	for i := range t.shards {
		sh := &t.shards[i]
		sh.mu.Lock()
		for k, fl := range sh.byKey {
			if fl.LastSeen.Before(cutoff) {
				delete(sh.byKey, k)
				continue
			}
			if f.UserID > 0 && k.UserID != f.UserID {
				continue
			}
			if f.Proto > 0 && int(k.Proto) != f.Proto {
				continue
			}
			if srcQ != "" && !strings.Contains(k.SrcIP, srcQ) {
				continue
			}
			if dstQ != "" && !strings.Contains(k.DstIP, dstQ) {
				continue
			}
			all = append(all, Snapshot{
				UserID: k.UserID, SrcIP: k.SrcIP, DstIP: k.DstIP,
				Proto: k.Proto, ProtoName: ProtoName(k.Proto),
				Sport: k.Sport, Dport: k.Dport,
				BytesIn: fl.BytesIn, BytesOut: fl.BytesOut,
				PacketsIn: fl.PacketsIn, PacketsOut: fl.PacketsOut,
				BytesTotal: fl.BytesIn + fl.BytesOut,
				FirstSeen: fl.FirstSeen, LastSeen: fl.LastSeen,
			})
		}
		sh.mu.Unlock()
	}

	keys := f.Sort
	if len(keys) == 0 {
		keys = []SortKey{{Key: SortLastSeen, Dir: "desc"}}
	}
	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i], all[j]
		for _, sk := range keys {
			cmp := compareSnapshot(a, b, sk.Key)
			if cmp == 0 {
				continue
			}
			if sk.Dir == "asc" {
				return cmp < 0
			}
			return cmp > 0
		}
		return false
	})

	total = len(all)
	if f.Offset < 0 {
		f.Offset = 0
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Offset >= total {
		return []Snapshot{}, total
	}
	end := f.Offset + f.Limit
	if end > total {
		end = total
	}
	return all[f.Offset:end], total
}

// compareSnapshot 按键比较；a<b 返回 -1，a>b 返回 1，相等 0。
func compareSnapshot(a, b Snapshot, key string) int {
	switch key {
	case SortRXBytes:
		return cmpInt64(a.BytesIn, b.BytesIn)
	case SortTXBytes:
		return cmpInt64(a.BytesOut, b.BytesOut)
	case SortPacketsIn:
		return cmpInt64(a.PacketsIn, b.PacketsIn)
	case SortPacketsOut:
		return cmpInt64(a.PacketsOut, b.PacketsOut)
	case SortLastSeen:
		if a.LastSeen.Before(b.LastSeen) {
			return -1
		}
		if a.LastSeen.After(b.LastSeen) {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func cmpInt64(a, b int64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
