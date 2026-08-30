package autostart

import (
	"fmt"
	"path/filepath"
	"strings"

	"haovpn/internal/brand"
)

// 跨平台自启产物名（纯常量；文档 / 单测 / 平台实现共用）。
const (
	// XDGDesktopFileName Linux「登录后起 GUI」桌面项文件名。
	XDGDesktopFileName = "haovpn-client-gui.desktop"

	// SystemdUnitName Linux「开机无界面」systemd 单元名。
	SystemdUnitName = "haovpn-client.service"

	// LaunchAgentLabel macOS 登录后起 GUI。
	LaunchAgentLabel = "com.haovpn.client.gui"

	// LaunchDaemonLabel macOS 开机无界面（须 root）。
	LaunchDaemonLabel = "com.haovpn.client"
)

// XDGAutostartDir 用户 XDG autostart 目录（通常 ~/.config/autostart）。
func XDGAutostartDir(home string) string {
	return filepath.Join(home, ".config", "autostart")
}

// XDGDesktopPath 登录自启 .desktop 完整路径。
func XDGDesktopPath(home string) string {
	return filepath.Join(XDGAutostartDir(home), XDGDesktopFileName)
}

// SystemdUnitPath 系统级 systemd unit 路径（须 root 可写）。
func SystemdUnitPath() string {
	return "/etc/systemd/system/" + SystemdUnitName
}

// LaunchAgentsDir 用户 LaunchAgents 目录。
func LaunchAgentsDir(home string) string {
	return filepath.Join(home, "Library", "LaunchAgents")
}

// LaunchAgentPlistPath 登录自启 plist 路径。
func LaunchAgentPlistPath(home string) string {
	return filepath.Join(LaunchAgentsDir(home), LaunchAgentLabel+".plist")
}

// LaunchDaemonPlistPath 开机守护进程 plist 路径。
func LaunchDaemonPlistPath() string {
	return "/Library/LaunchDaemons/" + LaunchDaemonLabel + ".plist"
}

// BuildXDGDesktopEntry 生成 XDG .desktop 正文（登录后起 GUI）。
//
// exe / configPath 应为绝对路径；configPath 空则不加 -c。
func BuildXDGDesktopEntry(exe, configPath string) string {
	execLine := shellQuote(exe)
	if strings.TrimSpace(configPath) != "" {
		execLine = shellQuote(exe) + " -c " + shellQuote(configPath)
	}
	var b strings.Builder
	b.WriteString("[Desktop Entry]\n")
	b.WriteString("Type=Application\n")
	b.WriteString("Version=1.0\n")
	fmt.Fprintf(&b, "Name=%s\n", brand.Name)
	b.WriteString("Comment=HaoVPN 客户端（登录后自启）\n")
	fmt.Fprintf(&b, "Exec=%s\n", execLine)
	b.WriteString("Terminal=false\n")
	b.WriteString("X-GNOME-Autostart-enabled=true\n")
	b.WriteString("StartupNotify=false\n")
	return b.String()
}

// BuildSystemdUnit 生成开机无界面 systemd unit（ExecStart 带 service 参数）。
func BuildSystemdUnit(exe, description string) string {
	if description == "" {
		description = brand.Name + " 客户端（开机无界面）"
	}
	var b strings.Builder
	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=%s\n", description)
	b.WriteString("After=network-online.target\n")
	b.WriteString("Wants=network-online.target\n")
	b.WriteString("\n[Service]\n")
	b.WriteString("Type=simple\n")
	// 路径含空格时须引号，否则 systemd 拆词失败
	fmt.Fprintf(&b, "ExecStart=%s service\n", shellQuote(exe))
	b.WriteString("Restart=on-failure\n")
	b.WriteString("RestartSec=5\n")
	b.WriteString("LimitNOFILE=65536\n")
	b.WriteString("\n[Install]\n")
	b.WriteString("WantedBy=multi-user.target\n")
	return b.String()
}

// BuildLaunchAgentPlist 生成 macOS 登录后起 GUI 的 LaunchAgent plist。
func BuildLaunchAgentPlist(exe, configPath string) string {
	args := []string{exe}
	if strings.TrimSpace(configPath) != "" {
		args = append(args, "-c", configPath)
	}
	return buildLaunchdPlist(LaunchAgentLabel, args, false)
}

// BuildLaunchDaemonPlist 生成 macOS 开机无界面 LaunchDaemon plist。
func BuildLaunchDaemonPlist(exe string) string {
	return buildLaunchdPlist(LaunchDaemonLabel, []string{exe, "service"}, true)
}

func buildLaunchdPlist(label string, programArgs []string, keepAlive bool) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")
	b.WriteString("\t<key>Label</key>\n")
	fmt.Fprintf(&b, "\t<string>%s</string>\n", xmlEscape(label))
	b.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	for _, a := range programArgs {
		fmt.Fprintf(&b, "\t\t<string>%s</string>\n", xmlEscape(a))
	}
	b.WriteString("\t</array>\n")
	b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n")
	if keepAlive {
		b.WriteString("\t<key>KeepAlive</key>\n\t<true/>\n")
	}
	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}

func shellQuote(s string) string {
	if s == "" {
		return `""`
	}
	if !strings.ContainsAny(s, " \t\"'\\") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func xmlEscape(s string) string {
	r := strings.NewReplacer(
		`&`, `&amp;`,
		`<`, `&lt;`,
		`>`, `&gt;`,
		`"`, `&quot;`,
		`'`, `&apos;`,
	)
	return r.Replace(s)
}
