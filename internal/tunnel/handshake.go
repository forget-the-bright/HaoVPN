package tunnel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"haovpn/internal/crypto"
	"haovpn/internal/fileutil"
)

// HandshakeRequest 客户端首帧握手 JSON（账号密码登录）。
//
// JSON 字段：
//   type — 固定为 "handshake"；其他值 ParseHandshakeRequest 拒绝。
//   username — 账号名；与 password 成对出现时为当前鉴权方式。
//   password — 账号密码；经 TLS 加密传输，服务端 VerifyTunnelLogin 校验。
//   public_key — 已废弃的公钥登录字段；非空时服务端拒绝并提示改用账号密码。
type HandshakeRequest struct {
	Type      string `json:"type"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	PublicKey string `json:"public_key,omitempty"` // 已废弃，服务端拒绝
}

// HandshakePolicy 服务端下发的运行时策略（权威配置，客户端必须以应答为准覆盖本地默认值）。
//
// JSON 字段：
//   vpn_ip — 分配给本连接的虚拟 IPv4 地址。
//   gateway_ip — TUN 网关地址（可选）；客户端配置路由/DNS 时使用。
//   allowed_ips — 客户端经 VPN 可访问的目的 CIDR 列表。
//   dns_servers — 推送给客户端的 DNS 服务器列表（可选）；空时客户端可回落 gateway_ip。
//   mtu — 隧道 MTU；客户端 TUN 与分片策略须与此一致。
//   ip_mode — IP 分配模式（fixed / dynamic_session / dynamic_lease）。
//   policy_ver — 策略版本号；服务端递增，变更后旧连接会被踢。
type HandshakePolicy struct {
	VPNIP      string   `json:"vpn_ip"`
	GatewayIP  string   `json:"gateway_ip,omitempty"`
	AllowedIPs []string `json:"allowed_ips"`
	DNSServers []string `json:"dns_servers,omitempty"`
	MTU        int      `json:"mtu"`
	IPMode     string   `json:"ip_mode"`
	PolicyVer  int      `json:"policy_ver"`
}

// HandshakeResponse 服务端握手应答 JSON。
//
// JSON 字段：
//   type — "handshake_ok" 成功 / "handshake_err" 失败。
//   server_public_key — 服务端 WireGuard 公钥（Base64）；客户端建立 crypto.Session 时使用。
//   client_private_key — 密码登录时下发的客户端私钥（可选）；仅 TLS 内传输，供客户端本地持久化。
//   policy — 成功时的运行时策略；失败时为 nil。
//   error — 失败时的可读错误信息；成功时为空。
type HandshakeResponse struct {
	Type             string           `json:"type"`
	ServerPublicKey  string           `json:"server_public_key"`
	ClientPrivateKey string           `json:"client_private_key,omitempty"` // 密码登录时下发，仅 TLS 内传输
	Policy           *HandshakePolicy `json:"policy,omitempty"`
	Error            string           `json:"error,omitempty"`
}

// EncodeHandshakeRequest 序列化客户端握手请求（旧：仅公钥，已废弃）。
//
// 参数：publicKey — WireGuard 公钥 Base64。
// 返回：JSON 字节；err 为 json.Marshal 失败。
// 副作用：无；服务端会拒绝公钥模式。
// 并发：任意 goroutine 可调用。
func EncodeHandshakeRequest(publicKey string) ([]byte, error) {
	req := HandshakeRequest{Type: "handshake", PublicKey: publicKey}
	return json.Marshal(req)
}

// EncodeHandshakeAuthRequest 序列化账号密码握手请求。
//
// 参数：username/password — 非空由调用方保证；经 TLS 传输。
// 返回：{"type":"handshake","username":...,"password":...} JSON；err 为 Marshal 失败。
// 副作用：无。
// 并发：client_handshake 拨号后调用。
func EncodeHandshakeAuthRequest(username, password string) ([]byte, error) {
	req := HandshakeRequest{Type: "handshake", Username: username, Password: password}
	return json.Marshal(req)
}

// ParseHandshakeRequest 解析客户端握手 JSON。
//
// 参数：data — 首帧 Handshake 载荷；须为合法 JSON。
// 返回：HandshakeRequest；err 为 JSON 无效、type≠handshake 或 username/public_key 均为空。
// 副作用：无。
// 并发：服务端 readLoop 回调中调用。
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
//
// 参数：serverPubKey — 服务端 WireGuard 公钥；policy — 权威运行时策略。
// 返回：handshake_ok JSON；err 为 Marshal 失败。
// 副作用：无；不含客户端私钥（见 EncodeHandshakeOKWithKey）。
// 并发：任意 goroutine 可调用。
func EncodeHandshakeOK(serverPubKey string, policy HandshakePolicy) ([]byte, error) {
	return EncodeHandshakeOKWithKey(serverPubKey, "", policy)
}

// EncodeHandshakeOKWithKey 握手成功并可附带客户端私钥（密码登录）。
//
// 参数：serverPubKey — 服务端公钥；clientPrivateKey — 可选，密码登录时下发的客户端私钥 Base64；
// policy — 运行时策略指针写入响应。
// 返回：handshake_ok JSON；err 为 Marshal 失败。
// 副作用：无；私钥仅经 TLS 内传输。
// 并发：server_handler doHandshake 成功路径调用。
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
//
// 参数：msg — 可读错误信息，写入 error 字段。
// 返回：{"type":"handshake_err","error":...} JSON；err 为 Marshal 失败。
// 副作用：无。
// 并发：rejectHandshake 调用。
func EncodeHandshakeErr(msg string) ([]byte, error) {
	resp := HandshakeResponse{Type: "handshake_err", Error: msg}
	return json.Marshal(resp)
}

// ParseHandshakeResponse 解析服务端握手响应（客户端用）。
//
// 参数：data — 服务端 Handshake 帧载荷。
// 返回：HandshakeResponse；err 为 JSON 无效。
// 副作用：无；不校验 type 字段，由调用方判断 handshake_ok/err。
// 并发：client_handshake 等待应答时调用。
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
	raw, _ := json.Marshal(ServerKeyFile{PublicKey: kp.PublicKey, PrivateKey: kp.PrivateKey})
	if err := fileutil.WriteFileAtomic(path, raw, 0o600); err != nil {
		return crypto.KeyPair{}, err
	}
	return kp, nil
}
