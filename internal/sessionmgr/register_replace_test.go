package sessionmgr

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"haovpn/internal/config"
	"haovpn/internal/crypto"
	"haovpn/internal/persist"
)

// fakePacketConn 记录 Close 调用；可选实现 Done / SetOnData 供排空路径单测。
type fakePacketConn struct {
	addr   string
	mu     sync.Mutex
	closed bool
	done   chan struct{}
	onData func([]byte)
}

func newFakePacketConn(addr string) *fakePacketConn {
	return &fakePacketConn{addr: addr, done: make(chan struct{})}
}

func (f *fakePacketConn) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.closed = true
		close(f.done)
	}
	return nil
}
func (f *fakePacketConn) RemoteAddr() string { return f.addr }
func (f *fakePacketConn) Send([]byte) error  { return nil }
func (f *fakePacketConn) LastPeerActivity() time.Time {
	return time.Now()
}
func (f *fakePacketConn) Done() <-chan struct{} { return f.done }
func (f *fakePacketConn) SetOnData(fn func([]byte)) {
	f.mu.Lock()
	f.onData = fn
	f.mu.Unlock()
}
func (f *fakePacketConn) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}
func (f *fakePacketConn) dataCallback() func([]byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.onData
}

// TestRegisterVPNClosesOldBeforeNewCrypto grace 顶替须先关旧 Conn，再挂新会话，
// 避免旧读循环同钥包占新防重放窗口（local_lans/ICS 长配网软重连现场）。
func TestRegisterVPNClosesOldBeforeNewCrypto(t *testing.T) {
	m := New(nil)
	m.SetSessionPolicy(config.SessionPolicyRejectSecond)
	m.SetReconnectGrace(60 * time.Second)

	srv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	cli, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	oldSess, err := crypto.NewSession(srv.PrivateKey, cli.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	newSess, err := crypto.NewSession(srv.PrivateKey, cli.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	oldConn := newFakePacketConn("1.2.3.4:1111")
	oldConn.SetOnData(func([]byte) {})
	newConn := newFakePacketConn("1.2.3.4:2222")
	user := &persist.User{ID: 9, Username: "u", PublicKey: cli.PublicKey, VPNIP: "10.88.0.9", IPMode: persist.IPModeFixed}

	if err := m.RegisterVPN(user, []string{"10.88.0.0/24"}, oldConn, oldSess, oldConn.addr, PeerReg{}); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterVPN(user, []string{"10.88.0.0/24"}, newConn, newSess, newConn.addr, PeerReg{}); err != nil {
		t.Fatal(err)
	}
	if !oldConn.isClosed() {
		t.Fatal("顶替后旧 Conn 应已被同步 Close")
	}
	if oldConn.dataCallback() != nil {
		t.Fatal("顶替后旧 Conn Data 回调应已清空")
	}
	select {
	case <-oldConn.Done():
	default:
		t.Fatal("顶替后旧 Conn Done 应已关闭")
	}
	m.mu.RLock()
	ps := m.sessions[user.ID]
	m.mu.RUnlock()
	if ps == nil || ps.Crypto != newSess {
		t.Fatal("新会话 Crypto 未挂上")
	}
	if ps.Conn != newConn {
		t.Fatal("新会话 Conn 未挂上")
	}
}

// TestHandleInboundDropsWhenSessionRemoved 会话已摘时入站须静默丢弃（顶替窗口）。
func TestHandleInboundDropsWhenSessionRemoved(t *testing.T) {
	m := New(nil)
	err := m.HandleInbound(99, nil, []byte("not-a-packet"), func([]byte) error {
		t.Fatal("不应写 TUN")
		return nil
	})
	if err != nil {
		t.Fatalf("无会话应返回 nil，got %v", err)
	}
}

// TestHandleInboundDropsStaleConn 旧 Conn 在新会话挂上后入站须静默丢弃且不烧新窗口。
//
// 复现 local_lans/ICS 软重连：同钥迟到包若按 userID 解密会占 counter=0..N，
// 导致新客户端 ascending replay。Conn 身份校验是第一道防线。
func TestHandleInboundDropsStaleConn(t *testing.T) {
	m := New(nil)
	m.SetSessionPolicy(config.SessionPolicyKickPrevious)

	srv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	cli, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	oldSess, err := crypto.NewSession(srv.PrivateKey, cli.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	newSess, err := crypto.NewSession(srv.PrivateKey, cli.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	oldConn := newFakePacketConn("1.2.3.4:1111")
	newConn := newFakePacketConn("1.2.3.4:2222")
	user := &persist.User{ID: 11, Username: "via", PublicKey: cli.PublicKey, VPNIP: "10.88.0.11", IPMode: persist.IPModeFixed}

	if err := m.RegisterVPN(user, []string{"10.88.0.0/24"}, oldConn, oldSess, oldConn.addr, PeerReg{}); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterVPN(user, []string{"10.88.0.0/24"}, newConn, newSess, newConn.addr, PeerReg{}); err != nil {
		t.Fatal(err)
	}

	// 旧客户端同钥密文（counter 从 0 起）；若误进新会话会烧窗口
	oldCli, err := crypto.NewSession(cli.PrivateKey, srv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	late, err := oldCli.Encrypt([]byte("late-from-old-conn"))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.HandleInbound(user.ID, oldConn, late, func([]byte) error {
		t.Fatal("陈旧 conn 不应写 TUN")
		return nil
	}); err != nil {
		t.Fatalf("陈旧 conn 应静默丢弃（nil），got %v", err)
	}

	// 新客户端 counter=0 须仍可通过（证明未烧窗口）
	newCli, err := crypto.NewSession(cli.PrivateKey, srv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := newCli.Encrypt(makeIPv4Packet(t, "10.88.0.11", "10.88.0.1"))
	if err != nil {
		t.Fatal(err)
	}
	wrote := false
	if err := m.HandleInbound(user.ID, newConn, fresh, func([]byte) error {
		wrote = true
		return nil
	}); err != nil {
		t.Fatalf("新 conn 合法包应解密成功: %v", err)
	}
	if !wrote {
		t.Fatal("新 conn 合法包应写入 TUN（AllowedIPs 含池）")
	}
}

// makeIPv4Packet 构造最小 IPv4 头（供入站校验用）。
func makeIPv4Packet(t *testing.T, src, dst string) []byte {
	t.Helper()
	pkt := make([]byte, 20)
	pkt[0] = 0x45
	sip := parseIPv4(t, src)
	dip := parseIPv4(t, dst)
	copy(pkt[12:16], sip)
	copy(pkt[16:20], dip)
	return pkt
}

func parseIPv4(t *testing.T, s string) []byte {
	t.Helper()
	ip := make([]byte, 4)
	var a, b, c, d int
	if _, err := fmt.Sscanf(s, "%d.%d.%d.%d", &a, &b, &c, &d); err != nil {
		t.Fatal(err)
	}
	ip[0], ip[1], ip[2], ip[3] = byte(a), byte(b), byte(c), byte(d)
	return ip
}
