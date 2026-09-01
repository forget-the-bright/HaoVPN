package crypto_test

import (
	"bytes"
	"fmt"
	"testing"

	"haovpn/internal/crypto"
)

// 现场「replay attack detected」可证伪验证：静态密钥 → 重连不换钥；防重放窗口却是新的。

func pairSessions(t *testing.T) (srvKP, cliKP crypto.KeyPair, sSrv, sCli *crypto.Session) {
	t.Helper()
	var err error
	srvKP, err = crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	cliKP, err = crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	sSrv, err = crypto.NewSession(srvKP.PrivateKey, cliKP.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	sCli, err = crypto.NewSession(cliKP.PrivateKey, srvKP.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return srvKP, cliKP, sSrv, sCli
}

// TestSameKeyAcrossReconnect 同一密钥对两次 NewSession，密钥材料互通（重连不换钥）。
func TestSameKeyAcrossReconnect(t *testing.T) {
	srvKP, cliKP, s1, c1 := pairSessions(t)
	s2, err := crypto.NewSession(srvKP.PrivateKey, cliKP.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := crypto.NewSession(cliKP.PrivateKey, srvKP.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := c1.Encrypt([]byte("from-old-client"))
	if err != nil {
		t.Fatal(err)
	}
	dec, err := s2.Decrypt(enc)
	if err != nil {
		t.Fatalf("新服务端会话应能解密旧客户端同钥密文: %v", err)
	}
	if !bytes.Equal(dec, []byte("from-old-client")) {
		t.Fatal("明文不匹配")
	}
	enc2, err := c2.Encrypt([]byte("from-new-client"))
	if err != nil {
		t.Fatal(err)
	}
	dec2, err := s1.Decrypt(enc2)
	if err != nil {
		t.Fatalf("旧服务端会话应能解密新客户端同钥密文: %v", err)
	}
	if !bytes.Equal(dec2, []byte("from-new-client")) {
		t.Fatal("明文不匹配")
	}
}

// TestStaleHighCounterPoisonsNewSession 旧高 counter 先进入新服务端窗口 → 新客户端从 0 起被拒。
func TestStaleHighCounterPoisonsNewSession(t *testing.T) {
	srvKP, cliKP, _, oldCli := pairSessions(t)
	const high = 600
	var latePkts [][]byte
	for i := 0; i < high; i++ {
		enc, err := oldCli.Encrypt([]byte(fmt.Sprintf("old-%d", i)))
		if err != nil {
			t.Fatal(err)
		}
		latePkts = append(latePkts, enc)
	}
	newSrv, err := crypto.NewSession(srvKP.PrivateKey, cliKP.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	newCli, err := crypto.NewSession(cliKP.PrivateKey, srvKP.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, enc := range latePkts {
		if _, err := newSrv.Decrypt(enc); err != nil {
			t.Fatalf("迟到旧包应能被新会话解密（同钥）: %v", err)
		}
	}
	fresh, err := newCli.Encrypt([]byte("fresh-after-reconnect"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = newSrv.Decrypt(fresh)
	if err == nil {
		t.Fatal("预期：迟到高 counter 毒化窗口后，新会话 counter=0 应被拒")
	}
	t.Logf("已证实毒化：%v", err)
}

// TestDecryptFailDoesNotBurnCounter Open 失败不得占用序号 → 同 counter 合法包仍可通过。
func TestDecryptFailDoesNotBurnCounter(t *testing.T) {
	_, _, sSrv, sCli := pairSessions(t)
	good, err := sCli.Encrypt([]byte("good-payload"))
	if err != nil {
		t.Fatal(err)
	}
	bad := append([]byte(nil), good...)
	bad[len(bad)-1] ^= 0xff
	if _, err := sSrv.Decrypt(bad); err == nil {
		t.Fatal("篡改包应 Open 失败")
	}
	dec, err := sSrv.Decrypt(good)
	if err != nil {
		t.Fatalf("Open 失败后同 counter 合法包应通过: %v", err)
	}
	if !bytes.Equal(dec, []byte("good-payload")) {
		t.Fatal("明文不匹配")
	}
}

// TestDualSenderSameKeyCollidingCounters 两路同钥从 0 计数打同一服务端 → 第二路 replay。
func TestDualSenderSameKeyCollidingCounters(t *testing.T) {
	srvKP, cliKP, sSrv, _ := pairSessions(t)
	cA, err := crypto.NewSession(cliKP.PrivateKey, srvKP.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	cB, err := crypto.NewSession(cliKP.PrivateKey, srvKP.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	encA, err := cA.Encrypt([]byte("from-A"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sSrv.Decrypt(encA); err != nil {
		t.Fatalf("A 首包: %v", err)
	}
	encB, err := cB.Encrypt([]byte("from-B"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sSrv.Decrypt(encB); err == nil {
		t.Fatal("B 与 A 同 counter=0，应 replay")
	}
}

// TestDuplicatePacketsAscendingReplay 每个 counter 成功后再重放 → 连续 ascending replay。
func TestDuplicatePacketsAscendingReplay(t *testing.T) {
	_, _, sSrv, sCli := pairSessions(t)
	const n = 50
	pkts := make([][]byte, 0, n)
	for i := 0; i < n; i++ {
		enc, err := sCli.Encrypt([]byte(fmt.Sprintf("p-%d", i)))
		if err != nil {
			t.Fatal(err)
		}
		pkts = append(pkts, enc)
		if _, err := sSrv.Decrypt(enc); err != nil {
			t.Fatalf("首包 counter=%d 应成功: %v", i, err)
		}
	}
	for i, enc := range pkts {
		if _, err := sSrv.Decrypt(enc); err == nil {
			t.Fatalf("重复包 counter=%d 应 replay", i)
		}
	}
}

// TestOldConnFeedsOverlappingCountersBlocksNewClient
// 旧连接把与新客户端重叠的 counter（含 0 起）灌进新会话 → 新客户端从 0 起被拒。
// 说明：仅灌入「远高于窗口」的序号才会靠「窗口外」拒包；同钥迟到包更常见的是「序号撞车」。
func TestOldConnFeedsOverlappingCountersBlocksNewClient(t *testing.T) {
	srvKP, cliKP, _, oldCli := pairSessions(t)
	var buffered [][]byte
	for i := 0; i < 30; i++ {
		enc, err := oldCli.Encrypt([]byte(fmt.Sprintf("overlap-%d", i)))
		if err != nil {
			t.Fatal(err)
		}
		buffered = append(buffered, enc)
	}
	newSrv, err := crypto.NewSession(srvKP.PrivateKey, cliKP.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	newCli, err := crypto.NewSession(cliKP.PrivateKey, srvKP.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, enc := range buffered {
		if _, err := newSrv.Decrypt(enc); err != nil {
			t.Fatalf("旧包进新会话: %v", err)
		}
	}
	for i := 0; i < 5; i++ {
		enc, err := newCli.Encrypt([]byte(fmt.Sprintf("new-%d", i)))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := newSrv.Decrypt(enc); err == nil {
			t.Fatalf("重叠 counter=%d 应被拒", i)
		}
	}
}

// TestMidRangeLatePacketsDoNotBlockCounterZero 仅迟到 mid-range 且未覆盖 0 时，
// 新客户端 counter=0 仍可通过（WG 滑动窗口约 2048）。用于否定「任意高序号都会挡 0」的过强假说。
func TestMidRangeLatePacketsDoNotBlockCounterZero(t *testing.T) {
	srvKP, cliKP, _, oldCli := pairSessions(t)
	for i := 0; i < 100; i++ {
		if _, err := oldCli.Encrypt([]byte("skip")); err != nil {
			t.Fatal(err)
		}
	}
	var mid [][]byte
	for i := 0; i < 20; i++ {
		enc, err := oldCli.Encrypt([]byte("mid"))
		if err != nil {
			t.Fatal(err)
		}
		mid = append(mid, enc)
	}
	newSrv, err := crypto.NewSession(srvKP.PrivateKey, cliKP.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	newCli, err := crypto.NewSession(cliKP.PrivateKey, srvKP.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, enc := range mid {
		if _, err := newSrv.Decrypt(enc); err != nil {
			t.Fatal(err)
		}
	}
	enc, err := newCli.Encrypt([]byte("zero-ok"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newSrv.Decrypt(enc); err != nil {
		t.Fatalf("未覆盖 counter=0 时新包应通过，实际: %v（若失败则窗口行为与文档假设不符）", err)
	}
}
