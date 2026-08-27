package tunnel

import (
	"fmt"
	"sync"
	"time"

	"haovpn/internal/crypto"
	"haovpn/internal/transport"
)

// HandshakeResult 客户端握手完整结果（含服务端下发策略与可选私钥）。
type HandshakeResult struct {
	ServerPublicKey  string
	ClientPrivateKey string
	Policy           HandshakePolicy
}

// ClientHandshake 客户端连接后发送握手并等待服务端应答。
type ClientHandshake struct {
	mu   sync.Mutex
	done bool
	res  HandshakeResult
	err  error
}

// NewClientHandshake 创建握手状态机。
func NewClientHandshake() *ClientHandshake {
	return &ClientHandshake{}
}

// RunAuthWithTimeout 账号密码握手。
func (c *ClientHandshake) RunAuthWithTimeout(conn *transport.Conn, username, password string, timeout time.Duration) (HandshakeResult, error) {
	req, err := EncodeHandshakeAuthRequest(username, password)
	if err != nil {
		return HandshakeResult{}, err
	}
	return c.runRaw(conn, req, timeout)
}

// RunWithTimeout 发送公钥握手并等待应答（旧模式兼容）。
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

	conn.SetOnData(func(data []byte) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.done || c.err != nil {
			return
		}
		resp, err := ParseHandshakeResponse(data)
		if err != nil {
			c.err = err
			finish()
			return
		}
		if resp.Type == "handshake_err" {
			c.err = fmt.Errorf("%s", resp.Error)
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

// Run 发送握手帧（默认 10s 超时）。
func (c *ClientHandshake) Run(conn *transport.Conn, peerPublicKey string) (HandshakeResult, error) {
	return c.RunWithTimeout(conn, peerPublicKey, 10*time.Second)
}

// BuildClientCrypto 握手成功后创建客户端加密会话。
func BuildClientCrypto(privateKey, serverPublicKey string) (*crypto.Session, error) {
	return crypto.NewSession(privateKey, serverPublicKey)
}
