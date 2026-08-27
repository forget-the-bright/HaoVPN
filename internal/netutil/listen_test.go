package netutil_test

import (
	"reflect"
	"testing"

	"haovpn/internal/netutil"
)

func TestAppendTunListenHost(t *testing.T) {
	tests := []struct {
		name  string
		hosts []string
		tunIP string
		want  []string
	}{
		{name: "默认追加 TUN 网关", hosts: []string{"127.0.0.1"}, tunIP: "10.88.0.1", want: []string{"127.0.0.1", "10.88.0.1"}},
		{name: "0.0.0.0 不再追加 TUN", hosts: []string{"0.0.0.0"}, tunIP: "10.88.0.1", want: []string{"0.0.0.0"}},
		{name: "IPv6 通配不再追加 TUN", hosts: []string{"::"}, tunIP: "10.88.0.1", want: []string{"::"}},
		{name: "已含 TUN IP 去重", hosts: []string{"127.0.0.1", "10.88.0.1"}, tunIP: "10.88.0.1", want: []string{"127.0.0.1", "10.88.0.1"}},
		{name: "空 TUN IP 不变", hosts: []string{"127.0.0.1"}, tunIP: "", want: []string{"127.0.0.1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := netutil.AppendTunListenHost(tc.hosts, tc.tunIP)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("AppendTunListenHost() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidatePublicBindPolicy(t *testing.T) {
	if err := netutil.ValidatePublicBindPolicy([]string{"127.0.0.1"}, false); err != nil {
		t.Fatal(err)
	}
	if err := netutil.ValidatePublicBindPolicy([]string{"0.0.0.0"}, false); err == nil {
		t.Fatal("应拒绝 wildcard 且 allowPublic=false")
	}
	if err := netutil.ValidatePublicBindPolicy([]string{"0.0.0.0"}, true); err != nil {
		t.Fatal(err)
	}
}

func TestResolveListenAddrsDefault(t *testing.T) {
	addrs, err := netutil.ResolveListenAddrs(nil, 8080)
	if err != nil || len(addrs) != 1 || addrs[0] != "127.0.0.1:8080" {
		t.Fatalf("ResolveListenAddrs: %v %v", addrs, err)
	}
}
