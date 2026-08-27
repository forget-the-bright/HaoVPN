// 临时：按 home 常用 SAN 重生自签证书。用法：go run ./scripts/regen_home_cert
package main

import (
	"fmt"
	"os"

	"haovpn/internal/security"
)

func main() {
	_ = os.Remove("./home/certs/server.crt")
	_ = os.Remove("./home/certs/server.key")
	err := security.EnsureServerCert("./home/certs/server.crt", "./home/certs/server.key", true, &security.CertGenOptions{
		ListenAddr: "0.0.0.0:8443",
		CertSANs:   []string{"192.168.196.17", "127.0.0.1", "localhost"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	fmt.Println("已重生 home/certs/server.crt（含 192.168.196.17）")
}
