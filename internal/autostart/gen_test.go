package autostart

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildXDGDesktopEntry(t *testing.T) {
	body := BuildXDGDesktopEntry("/opt/haovpn/haovpn-client-gui", "/etc/haovpn/client.yaml")
	for _, want := range []string{
		"[Desktop Entry]",
		"Type=Application",
		"Exec=/opt/haovpn/haovpn-client-gui -c /etc/haovpn/client.yaml",
		"X-GNOME-Autostart-enabled=true",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
	quoted := BuildXDGDesktopEntry("/opt/Hao VPN/gui", "")
	if !strings.Contains(quoted, `"/opt/Hao VPN/gui"`) {
		t.Fatalf("expected quoted path:\n%s", quoted)
	}
}

func TestBuildSystemdUnit(t *testing.T) {
	body := BuildSystemdUnit("/usr/local/bin/haovpn-client-gui", "")
	for _, want := range []string{
		"[Unit]",
		"ExecStart=/usr/local/bin/haovpn-client-gui service",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
	quoted := BuildSystemdUnit("/opt/Hao VPN/haovpn-client", "")
	if !strings.Contains(quoted, `ExecStart="/opt/Hao VPN/haovpn-client" service`) {
		t.Fatalf("路径含空格须引号:\n%s", quoted)
	}
	if SystemdUnitName == "" {
		t.Fatal("empty SystemdUnitName")
	}
	if SystemdUnitPath() != "/etc/systemd/system/"+SystemdUnitName {
		t.Fatalf("unexpected unit path %s", SystemdUnitPath())
	}
}

func TestBuildLaunchdPlists(t *testing.T) {
	agent := BuildLaunchAgentPlist("/Applications/HaoVPN.app/Contents/MacOS/gui", "/tmp/c.yaml")
	for _, want := range []string{
		LaunchAgentLabel,
		"<string>/Applications/HaoVPN.app/Contents/MacOS/gui</string>",
		"<string>-c</string>",
		"<string>/tmp/c.yaml</string>",
		"<key>RunAtLoad</key>",
	} {
		if !strings.Contains(agent, want) {
			t.Fatalf("agent missing %q in:\n%s", want, agent)
		}
	}
	daemon := BuildLaunchDaemonPlist("/usr/local/bin/haovpn-client")
	for _, want := range []string{
		LaunchDaemonLabel,
		"<string>service</string>",
		"<key>KeepAlive</key>",
	} {
		if !strings.Contains(daemon, want) {
			t.Fatalf("daemon missing %q in:\n%s", want, daemon)
		}
	}
	home := "/Users/test"
	wantAgent := filepath.Join(home, "Library", "LaunchAgents", LaunchAgentLabel+".plist")
	if LaunchAgentPlistPath(home) != wantAgent {
		t.Fatalf("agent path: got %s want %s", LaunchAgentPlistPath(home), wantAgent)
	}
}

func TestXMLEscape(t *testing.T) {
	got := xmlEscape(`a&b<c>"d"`)
	if !strings.Contains(got, "&amp;") || !strings.Contains(got, "&lt;") {
		t.Fatalf("escape failed: %s", got)
	}
}
