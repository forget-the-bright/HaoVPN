package flowmon

import (
	"net"
	"sync"
	"testing"
	"time"
)

// TestObserveMergeDirections 入站/出站应合并为同一流并分别累计字节。
func TestObserveMergeDirections(t *testing.T) {
	tr := New(Options{MaxPerUser: 100})
	pktIn := buildIPv4UDP("10.88.0.2", "192.168.3.1", 1234, 53, 32)
	tr.ObservePacket(7, pktIn, DirIn)
	pktOut := buildIPv4UDP("192.168.3.1", "10.88.0.2", 53, 1234, 48)
	tr.ObservePacket(7, pktOut, DirOut)

	items, total := tr.List(ListFilter{UserID: 7, Limit: 10})
	if total != 1 {
		t.Fatalf("total=%d want 1 items=%v", total, items)
	}
	f := items[0]
	if f.DstIP != "192.168.3.1" || f.SrcIP != "10.88.0.2" {
		t.Fatalf("canonical endpoints %+v", f)
	}
	if f.BytesIn != 32 || f.BytesOut != 48 {
		t.Fatalf("bytes in=%d out=%d", f.BytesIn, f.BytesOut)
	}
	if f.ProtoName != "UDP" || f.Sport != 1234 || f.Dport != 53 {
		t.Fatalf("l4 %+v", f)
	}
}

// TestMaxPerUserAcrossShards 每用户上限按全表合计，非单分片。
func TestMaxPerUserAcrossShards(t *testing.T) {
	tr := New(Options{MaxPerUser: 3, TTL: time.Hour})
	uid := int64(42)
	for i := 0; i < 20; i++ {
		dst := net.IPv4(10, 0, byte(i>>8), byte(i)).String()
		pkt := buildIPv4UDP("10.88.0.2", dst, uint16(1000+i), 80, 40)
		tr.ObservePacket(uid, pkt, DirIn)
	}
	_, total := tr.List(ListFilter{UserID: uid, Limit: 100})
	if total != 3 {
		t.Fatalf("跨分片合计应 ==3, got total=%d", total)
	}
}

// TestMaxPerUserConcurrent 并发插入不得突破 maxPerUser。
func TestMaxPerUserConcurrent(t *testing.T) {
	tr := New(Options{MaxPerUser: 5, TTL: time.Hour})
	uid := int64(99)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				dst := net.IPv4(10, byte(g), byte(i>>8), byte(i)).String()
				pkt := buildIPv4UDP("10.88.0.2", dst, uint16(2000+g*100+i), 443, 40)
				tr.ObservePacket(uid, pkt, DirIn)
			}
		}(g)
	}
	wg.Wait()
	_, total := tr.List(ListFilter{UserID: uid, Limit: 100})
	if total > 5 {
		t.Fatalf("并发后应 ≤5, got %d", total)
	}
	if total < 1 {
		t.Fatal("应至少保留流")
	}
}

// TestObserveTTLPurge Observe 低频淘汰过期流（不等 List）。
func TestObserveTTLPurge(t *testing.T) {
	tr := New(Options{TTL: 30 * time.Millisecond, MaxPerUser: 100, ObserveTTLEvery: 1})
	pkt := buildIPv4UDP("10.88.0.2", "1.1.1.1", 1000, 53, 40)
	tr.ObservePacket(9, pkt, DirIn)
	items, _ := tr.List(ListFilter{UserID: 9, Limit: 10})
	if len(items) != 1 {
		t.Fatal("应先有一条流")
	}
	first := items[0].FirstSeen
	time.Sleep(50 * time.Millisecond)
	// 同一 key：先按 TTL 删掉过期再插入，FirstSeen 应刷新
	tr.ObservePacket(9, pkt, DirIn)
	items, total := tr.List(ListFilter{UserID: 9, Limit: 10})
	if total != 1 {
		t.Fatalf("total=%d", total)
	}
	if !items[0].FirstSeen.After(first) {
		t.Fatalf("过期流应已淘汰重建 first=%v now=%v", first, items[0].FirstSeen)
	}
}

// TestListPagination total 与 limit/offset。
func TestListPagination(t *testing.T) {
	tr := New(Options{MaxPerUser: 100})
	for i := 0; i < 5; i++ {
		pkt := buildIPv4UDP("10.88.0.2", "10.0.0.1", 1000, uint16(80+i), 40)
		tr.ObservePacket(1, pkt, DirIn)
	}
	page, total := tr.List(ListFilter{UserID: 1, Limit: 2, Offset: 0})
	if total != 5 || len(page) != 2 {
		t.Fatalf("page0 len=%d total=%d", len(page), total)
	}
	page2, _ := tr.List(ListFilter{UserID: 1, Limit: 2, Offset: 4})
	if len(page2) != 1 {
		t.Fatalf("page2 len=%d", len(page2))
	}
}

// TestNonIPv4Dropped 非 IPv4 丢弃。
func TestNonIPv4Dropped(t *testing.T) {
	tr := New(Options{})
	b := make([]byte, 40)
	b[0] = 0x60 // IPv6
	tr.ObservePacket(1, b, DirIn)
	_, total := tr.List(ListFilter{Limit: 10})
	if total != 0 {
		t.Fatalf("want 0 got %d", total)
	}
}

// TestICMPNoPorts ICMP 无端口仍记流。
func TestICMPNoPorts(t *testing.T) {
	tr := New(Options{})
	b := make([]byte, 28)
	b[0] = 0x45
	b[9] = 1
	copy(b[12:16], net.ParseIP("10.88.0.2").To4())
	copy(b[16:20], net.ParseIP("8.8.8.8").To4())
	tr.ObservePacket(3, b, DirIn)
	items, total := tr.List(ListFilter{UserID: 3, Limit: 5})
	if total != 1 || items[0].ProtoName != "ICMP" || items[0].Sport != 0 {
		t.Fatalf("%+v total=%d", items, total)
	}
}

// TestListFilterByIP src_ip / dst_ip 子串过滤。
func TestListFilterByIP(t *testing.T) {
	tr := New(Options{MaxPerUser: 100})
	tr.ObservePacket(1, buildIPv4UDP("10.88.0.2", "192.168.3.1", 1000, 53, 40), DirIn)
	tr.ObservePacket(1, buildIPv4UDP("10.88.0.2", "8.8.8.8", 1001, 53, 40), DirIn)
	_, total := tr.List(ListFilter{UserID: 1, DstIP: "192.168", Limit: 10})
	if total != 1 {
		t.Fatalf("dst filter total=%d want 1", total)
	}
	_, total = tr.List(ListFilter{UserID: 1, SrcIP: "10.88", Limit: 10})
	if total != 2 {
		t.Fatalf("src filter total=%d want 2", total)
	}
}

// TestListMultiSort rx_bytes desc 再 last_seen。
func TestListMultiSort(t *testing.T) {
	tr := New(Options{MaxPerUser: 100})
	// 小字节
	tr.ObservePacket(1, buildIPv4UDP("10.88.0.2", "1.1.1.1", 2000, 80, 40), DirIn)
	// 大字节（更长包）
	tr.ObservePacket(1, buildIPv4UDP("10.88.0.2", "1.1.1.2", 2001, 80, 200), DirIn)
	items, total := tr.List(ListFilter{
		UserID: 1, Limit: 10,
		Sort: []SortKey{{Key: SortRXBytes, Dir: "desc"}},
	})
	if total != 2 || len(items) != 2 {
		t.Fatalf("total=%d len=%d", total, len(items))
	}
	if items[0].BytesIn < items[1].BytesIn {
		t.Fatalf("rx desc: first=%d second=%d", items[0].BytesIn, items[1].BytesIn)
	}
	if items[0].DstIP != "1.1.1.2" {
		t.Fatalf("want larger packet first, got dst=%s", items[0].DstIP)
	}
}

// TestParseSortQuery 解析与非法键跳过。
func TestParseSortQuery(t *testing.T) {
	keys := ParseSortQuery("rx_bytes:desc,last_seen:asc,bad:xx")
	if len(keys) != 2 || keys[0].Key != SortRXBytes || keys[0].Dir != "desc" ||
		keys[1].Key != SortLastSeen || keys[1].Dir != "asc" {
		t.Fatalf("%+v", keys)
	}
}

func buildIPv4UDP(src, dst string, sport, dport uint16, totalLen int) []byte {
	if totalLen < 28 {
		totalLen = 28
	}
	b := make([]byte, totalLen)
	b[0] = 0x45
	b[9] = 17
	copy(b[12:16], net.ParseIP(src).To4())
	copy(b[16:20], net.ParseIP(dst).To4())
	b[20] = byte(sport >> 8)
	b[21] = byte(sport)
	b[22] = byte(dport >> 8)
	b[23] = byte(dport)
	return b
}
