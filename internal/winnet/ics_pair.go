package winnet

import (
	"strings"
	"sync"
)

// 本会话 ICS 网卡对（setupICSPlatform 成功后写入；Teardown 读取后清空）。
var (
	icsPairMu      sync.Mutex
	icsPairPublic  string
	icsPairPrivate string
)

// RememberICSPair 记录本会话启用的 ICS public/private 网卡名，供退出时靶向关闭。
func RememberICSPair(public, private string) {
	icsPairMu.Lock()
	defer icsPairMu.Unlock()
	icsPairPublic = strings.TrimSpace(public)
	icsPairPrivate = strings.TrimSpace(private)
}

// TakeICSPair 取出并清空本会话 ICS 网卡对；未记录时 ok=false。
func TakeICSPair() (public, private string, ok bool) {
	icsPairMu.Lock()
	defer icsPairMu.Unlock()
	public, private = icsPairPublic, icsPairPrivate
	icsPairPublic, icsPairPrivate = "", ""
	ok = public != "" || private != ""
	return public, private, ok
}

// ClearICSPair 清空记录（测试或未走 Take 的路径）。
func ClearICSPair() {
	icsPairMu.Lock()
	defer icsPairMu.Unlock()
	icsPairPublic, icsPairPrivate = "", ""
}
