// Package tunnel 实现 VPN 隧道握手协议与服务端隧道密钥持久化。
package tunnel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"haovpn/internal/crypto"
)

// HandshakeRequest 客户端首帧握手（仅 username+password）。
type HandshakeRequest struct {
	Type      string `json:"type"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	PublicKey string `json:"public_key,omitempty"` // 已废弃，服务端拒绝
}

// HandshakePolicy 服务端下发的运行时策略（权威，客户端必须以应答为准）。
type HandshakePolicy struct {
	VPNIP      string   `json:"vpn_ip"`
	GatewayIP  string   `json:"gateway_ip,omitempty"`
	AllowedIPs []string `json:"allowed_ips"`
	DNSServers []string `json:"dns_servers,omitempty"`
	MTU        int      `json:"mtu"`
	IPMode     string   `json:"ip_mode"`
	PolicyVer  int      `json:"policy_ver"`
}

// HandshakeResponse 服务端握手应答。
type HandshakeResponse struct {
	Type             string           `json:"type"`
	ServerPublicKey  string           `json:"server_public_key"`
	ClientPrivateKey string           `json:"client_private_key,omitempty"` // 密码登录时下发，仅 TLS 内传输
	Policy           *HandshakePolicy `json:"policy,omitempty"`
	Error            string           `json:"error,omitempty"`
}

// EncodeHandshakeRequest 序列化客户端握手请求（旧：仅公钥）。
func EncodeHandshakeRequest(publicKey string) ([]byte, error) {
	req := HandshakeRequest{Type: "handshake", PublicKey: publicKey}
	return json.Marshal(req)
}

// EncodeHandshakeAuthRequest 序列化账号密码握手请求。
func EncodeHandshakeAuthRequest(username, password string) ([]byte, error) {
	req := HandshakeRequest{Type: "handshake", Username: username, Password: password}
	return json.Marshal(req)
}

// ParseHandshakeRequest 解析客户端握手 JSON。
func ParseHandshakeRequest(data []byte) (HandshakeRequest, error) {
	var req HandshakeRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return req, fmt.Errorf("解析握手 JSON: %w", err)
	}
	if req.Type != "handshake" {
		return req, fmt.Errorf("无效握手请求")
	}
	if req.Username == "" && req.PublicKey == "" {
		return req, fmt.Errorf("无效握手请求: 须提供 username 或 public_key")
	}
	return req, nil
}

// EncodeHandshakeOK 序列化握手成功响应（含运行时策略）。
func EncodeHandshakeOK(serverPubKey string, policy HandshakePolicy) ([]byte, error) {
	return EncodeHandshakeOKWithKey(serverPubKey, "", policy)
}

// EncodeHandshakeOKWithKey 握手成功并可附带客户端私钥（密码登录）。
func EncodeHandshakeOKWithKey(serverPubKey, clientPrivateKey string, policy HandshakePolicy) ([]byte, error) {
	resp := HandshakeResponse{
		Type:             "handshake_ok",
		ServerPublicKey:  serverPubKey,
		ClientPrivateKey: clientPrivateKey,
		Policy:           &policy,
	}
	return json.Marshal(resp)
}

// EncodeHandshakeErr 序列化握手失败响应。
func EncodeHandshakeErr(msg string) ([]byte, error) {
	resp := HandshakeResponse{Type: "handshake_err", Error: msg}
	return json.Marshal(resp)
}

// ParseHandshakeResponse 解析服务端握手响应（客户端用）。
func ParseHandshakeResponse(data []byte) (HandshakeResponse, error) {
	var resp HandshakeResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return resp, err
	}
	return resp, nil
}

// KeyFile 隧道密钥文件路径默认名。
const KeyFileName = "tunnel_key.json"

// ServerKeyFile 读写服务端隧道密钥对（JSON）。
type ServerKeyFile struct {
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

// LoadOrCreateServerKeys 从目录加载或生成服务端隧道密钥。
func LoadOrCreateServerKeys(dir string) (crypto.KeyPair, error) {
	path := filepath.Join(dir, KeyFileName)
	data, err := os.ReadFile(path)
	if err == nil {
		var kf ServerKeyFile
		if err := json.Unmarshal(data, &kf); err != nil {
			return crypto.KeyPair{}, fmt.Errorf("解析隧道密钥: %w", err)
		}
		if kf.PublicKey == "" || kf.PrivateKey == "" {
			return crypto.KeyPair{}, fmt.Errorf("隧道密钥文件字段不完整")
		}
		return crypto.KeyPair{PublicKey: kf.PublicKey, PrivateKey: kf.PrivateKey}, nil
	}
	if !os.IsNotExist(err) {
		return crypto.KeyPair{}, err
	}
	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		return crypto.KeyPair{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return crypto.KeyPair{}, err
	}
	raw, _ := json.Marshal(ServerKeyFile{PublicKey: kp.PublicKey, PrivateKey: kp.PrivateKey})
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return crypto.KeyPair{}, err
	}
	return kp, nil
}
