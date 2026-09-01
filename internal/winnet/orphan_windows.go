//go:build windows

package winnet

import (
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"haovpn/internal/logger"
)

// HasWintunOrphanAdapters 用 GetAdaptersAddresses 检测是否存在同名前缀孤儿网卡。
//
// 无孤儿时 Open 失败路径可跳过冷启 PowerShell（公司机可省数秒）。
// 探测失败时返回 true（保守：仍跑清理脚本）。
func HasWintunOrphanAdapters(configName string) bool {
	start := time.Now()
	if strings.TrimSpace(configName) == "" {
		return false
	}
	names, err := listAdapterNamesAndDescs()
	if err != nil {
		logger.Warn("prepare_orphan probe fail elapsed=%s: %v（保守视为有孤儿）", time.Since(start), err)
		return true
	}
	for _, a := range names {
		if IsWintunOrphanAdapterName(configName, a.name, a.desc) {
			logger.Info("prepare_orphan probe elapsed=%s hit=true name=%q", time.Since(start), a.name)
			return true
		}
	}
	logger.Info("prepare_orphan probe elapsed=%s hit=false", time.Since(start))
	return false
}

type adapterNameDesc struct {
	name string
	desc string
}

func listAdapterNamesAndDescs() ([]adapterNameDesc, error) {
	var buf []byte
	size := uint32(15000)
	var err error
	for i := 0; i < 3; i++ {
		buf = make([]byte, size)
		// AF_UNSPEC：含无 IPv4 的适配器；孤儿可能尚未配地址
		err = windows.GetAdaptersAddresses(windows.AF_UNSPEC,
			windows.GAA_FLAG_SKIP_ANYCAST|windows.GAA_FLAG_SKIP_MULTICAST|windows.GAA_FLAG_SKIP_DNS_SERVER,
			0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if err == windows.ERROR_BUFFER_OVERFLOW {
			continue
		}
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	var out []adapterNameDesc
	aa := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
	for ; aa != nil; aa = aa.Next {
		name := windows.UTF16PtrToString(aa.FriendlyName)
		desc := windows.UTF16PtrToString(aa.Description)
		if name == "" {
			continue
		}
		out = append(out, adapterNameDesc{name: name, desc: desc})
	}
	return out, nil
}
