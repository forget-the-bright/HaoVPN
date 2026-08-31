package tunnel

import (
	"fmt"
	"sync"
	"time"

	"haovpn/internal/autherr"
	"haovpn/internal/crypto"
	"haovpn/internal/transport"
)

// HandshakeResult 客户端握手成功后的完整结果（含服务端策略与可选下发的私钥）。
//
// 字段：
//   ServerPublicKey — 服务端隧道公钥；BuildClientCrypto 时使用。
//   ClientPrivateKey — 密码登录时服务端下发的私钥；空表示客户端已有本地密钥。
//   Policy — 权威运行时策略；须覆盖本地 TUN/路由/DNS/MTU 配置。
type HandshakeResult struct {
	ServerPublicKey  string
	ClientPrivateKey string
	Policy           HandshakePolicy
}

// ClientHandshake 客户端握手状态机；在 transport.Conn 上发送请求并等待首帧应答。
//
// 字段：
//   mu — 保护 done/res/err；OnData 回调与 Run* 方法并发访问。
//   done — 是否已收到 handshake_ok。
//   res — 成功时的 HandshakeResult。
//   err — 解析失败、handshake_err 或超时错误。
type ClientHandshake struct {
	mu   sync.Mutex
	done bool
	res  HandshakeResult
	err  error
}

// NewClientHandshake 创建空的客户端握手状态机。
//
// 返回：可复用于单次 Run* 调用的 *ClientHandshake；每次连接应新建实例。
func NewClientHandshake() *ClientHandshake {
	return &ClientHandshake{}
}

// RunAuthWithTimeout 发送账号密码握手并阻塞等待应答（无 local_lans）。
func (c *ClientHandshake) RunAuthWithTimeout(conn *transport.Conn, username, password string, timeout time.Duration) (HandshakeResult, error) {
	return c.RunAuthWithTimeoutEx(conn, username, password, nil, "", timeout)
}

// RunAuthWithTimeoutEx 发送账号密码握手，可附带 local_lans / host_id。
func (c *ClientHandshake) RunAuthWithTimeoutEx(conn *transport.Conn, username, password string, localLans []string, hostID string, timeout time.Duration) (HandshakeResult, error) {
	req, err := EncodeHandshakeAuthRequestEx(username, password, localLans, hostID)
	if err != nil {
		return HandshakeResult{}, err
	}
	return c.runRaw(conn, req, timeout)
}

// RunWithTimeout 发送公钥握手并等待应答（已废弃模式，服务端会拒绝）。
//
// 参数：
//   conn — 已建立 TLS 的传输连接。
//   peerPublicKey — 客户端公钥字符串（旧协议字段）。
//   timeout — 等待应答的最长时间。
//
// 返回：同 RunAuthWithTimeout；当前服务端要求账号密码，通常得到 handshake_err。
func (c *ClientHandshake) RunWithTimeout(conn *transport.Conn, peerPublicKey string, timeout time.Duration) (HandshakeResult, error) {
	req, err := EncodeHandshakeRequest(peerPublicKey)
	if err != nil {
		return HandshakeResult{}, err
	}
	return c.runRaw(conn, req, timeout)
}

func (c *ClientHandshake) runRaw(conn *transport.Conn, req []byte, timeout time.Duration) (HandshakeResult, error) {
	done := make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(done) }) }

	// 仅挂 Handshake 回调：Data 密文常以 \\x00 开头，不可走 json.Unmarshal。
	conn.SetOnHandshake(func(data []byte) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.done || c.err != nil {
			return
		}
		resp, err := ParseHandshakeResponse(data)
		if err != nil {
			prefix := data
			if len(prefix) > 8 {
				prefix = prefix[:8]
			}
			c.err = fmt.Errorf("握手应答非 JSON len=%d head=%x: %w", len(data), prefix, err)
			finish()
			return
		}
		if resp.Type == "handshake_err" {
			// 优先用稳定 code 还原哨兵，保留 errors.Is；无 code 时退回文案（兼容旧服务端）。
			c.err = handshakeErrFromResponse(resp)
			finish()
			return
		}
		if resp.Type == "handshake_ok" {
			c.res.ServerPublicKey = resp.ServerPublicKey
			c.res.ClientPrivateKey = resp.ClientPrivateKey
			if resp.Policy != nil {
				c.res.Policy = *resp.Policy
			}
			c.done = true
			finish()
		}
	})

	if err := conn.SendRaw(transport.FrameTypeHandshake, req); err != nil {
		return HandshakeResult{}, err
	}

	select {
	case <-done:
	case <-time.After(timeout):
		return HandshakeResult{}, fmt.Errorf("握手超时")
	}

	conn.SetOnHandshake(nil) // 鉴权结束，后续只走 SetOnData

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return HandshakeResult{}, c.err
	}
	if !c.done {
		return HandshakeResult{}, fmt.Errorf("握手未完成")
	}
	return c.res, nil
}

// handshakeErrFromResponse 从 handshake_err 应答还原可 errors.Is 的错误。
//
// 优先 autherr.FromHandshakeCode；无 code 时用 Error 文案构造（旧服务端兼容）。
// 有 code 且文案不同时：wrap 哨兵并附带服务端原文，便于日志。
func handshakeErrFromResponse(resp HandshakeResponse) error {
	if sent := autherr.FromHandshakeCode(resp.Code); sent != nil {
		if resp.Error == "" || resp.Error == sent.Error() {
			return sent
		}
		return fmt.Errorf("%s: %w", resp.Error, sent)
	}
	if resp.Error != "" {
		return fmt.Errorf("%s", resp.Error)
	}
	return fmt.Errorf("握手失败")
}

// Run 发送公钥握手并等待应答（默认 10s 超时）。
//
// 参数：conn — 传输连接；peerPublicKey — 客户端公钥（旧模式）。
// 返回：同 RunWithTimeout。
func (c *ClientHandshake) Run(conn *transport.Conn, peerPublicKey string) (HandshakeResult, error) {
	return c.RunWithTimeout(conn, peerPublicKey, 10*time.Second)
}

// BuildClientCrypto 握手成功后创建客户端加密会话。
func BuildClientCrypto(privateKey, serverPublicKey string) (*crypto.Session, error) {
	return crypto.NewSession(privateKey, serverPublicKey)
}
