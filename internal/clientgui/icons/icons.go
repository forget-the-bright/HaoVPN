// Package icons 嵌入 HaoVPN 托盘多状态图标与品牌 Logo。
//
// 品牌语义（无文字）：对勾路径 =「好/通路确认」；节点连线 = VPN 组网。
package icons

import (
	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

//go:embed tray_idle.png
var idlePNG []byte

//go:embed tray_connecting.png
var connectingPNG []byte

//go:embed tray_connected.png
var connectedPNG []byte

//go:embed tray_error.png
var errorPNG []byte

//go:embed logo.png
var logoPNG []byte

func mustResource(name string, data []byte) fyne.Resource {
	return fyne.NewStaticResource(name, data)
}

// Idle 灰色托盘（未连接）。
var Idle = mustResource("tray_idle.png", idlePNG)

// Connecting 黄色托盘（连接中/重连中）。
var Connecting = mustResource("tray_connecting.png", connectingPNG)

// Connected 绿色托盘（已连接）。
var Connected = mustResource("tray_connected.png", connectedPNG)

// Error 红色托盘（有错误）。
var Error = mustResource("tray_error.png", errorPNG)

// Logo HaoVPN 品牌标（对勾路径 + 节点；无文字）。
var Logo = mustResource("logo.png", logoPNG)

// LogoImage 返回可放入布局的 Logo 画布对象。
func LogoImage() *canvas.Image {
	img := canvas.NewImageFromResource(Logo)
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(48, 48))
	return img
}
