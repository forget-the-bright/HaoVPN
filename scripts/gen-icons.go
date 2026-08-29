//go:build ignore

// 从品牌源图生成 Logo / appicon，并为托盘绘制简化「对勾路径 + 节点」多状态图标。
// 概念：好 = 通路确认（checkmark）；网络/VPN = 节点与连线。无文字。
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

func main() {
	root := `d:\project\private\go\go-vpn`
	srcPath := filepath.Join(root, "assets", "haovpn-logo-source.png")
	f, err := os.Open(srcPath)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		panic(err)
	}

	iconsDir := filepath.Join(root, "internal", "clientgui", "icons")
	assetsDir := filepath.Join(root, "assets")

	logo256 := resizeBilinear(src, 256)
	mustWritePNG(filepath.Join(iconsDir, "logo.png"), logo256)
	mustWritePNG(filepath.Join(assetsDir, "appicon.png"), resizeBilinear(src, 512))
	mustWritePNG(filepath.Join(assetsDir, "appicon-256.png"), logo256)
	mustWritePNG(filepath.Join(root, "cmd", "client-gui", "Icon.png"), resizeBilinear(src, 512))

	// 托盘：状态色底 + 简化几何标（小尺寸可读）
	variants := []struct {
		name string
		bg   color.RGBA
		mark color.RGBA
	}{
		{"tray_idle.png", color.RGBA{88, 96, 108, 255}, color.RGBA{220, 230, 240, 255}},
		{"tray_connecting.png", color.RGBA{185, 130, 20, 255}, color.RGBA{255, 240, 160, 255}},
		{"tray_connected.png", color.RGBA{18, 120, 95, 255}, color.RGBA{120, 255, 210, 255}},
		{"tray_error.png", color.RGBA{170, 45, 50, 255}, color.RGBA{255, 210, 210, 255}},
	}
	for _, v := range variants {
		hi := drawVPNMark(128, v.bg, v.mark)
		mustWritePNG(filepath.Join(iconsDir, v.name), resizeBilinear(hi, 64))
	}
}

// drawVPNMark 绘制「对勾路径 + 卫星节点」：表达「好/通路确认」与 VPN 组网。
func drawVPNMark(size int, bg, mark color.RGBA) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, size, size))
	cx, cy := float64(size)/2, float64(size)/2
	rOuter := float64(size)/2 - 1

	// 圆底（软边）
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			if d <= rOuter-1 {
				out.Set(x, y, bg)
			} else if d < rOuter+0.4 {
				t := rOuter + 0.4 - d
				if t > 1 {
					t = 1
				}
				out.Set(x, y, color.RGBA{bg.R, bg.G, bg.B, uint8(t * 255)})
			}
		}
	}

	// 三卫星节点（VPN peer）
	nodes := [][2]float64{
		{cx - float64(size)*0.28, cy - float64(size)*0.22},
		{cx + float64(size)*0.30, cy - float64(size)*0.18},
		{cx + float64(size)*0.08, cy + float64(size)*0.30},
	}
	nodeR := float64(size) * 0.055
	stroke := float64(size) * 0.055

	// 弧线连接节点（网状）
	line(out, nodes[0][0], nodes[0][1], nodes[1][0], nodes[1][1], stroke*0.55, withAlpha(mark, 140))
	line(out, nodes[1][0], nodes[1][1], nodes[2][0], nodes[2][1], stroke*0.55, withAlpha(mark, 140))
	line(out, nodes[2][0], nodes[2][1], nodes[0][0], nodes[0][1], stroke*0.45, withAlpha(mark, 100))

	for _, n := range nodes {
		fillCircle(out, n[0], n[1], nodeR, mark)
	}

	// 中央对勾（「好」= 确认通路）：粗描边，略偏左下→右上
	// 短臂
	ax, ay := cx-float64(size)*0.18, cy+float64(size)*0.02
	bx, by := cx-float64(size)*0.02, cy+float64(size)*0.18
	cx2, cy2 := cx+float64(size)*0.26, cy-float64(size)*0.16
	line(out, ax, ay, bx, by, stroke, mark)
	line(out, bx, by, cx2, cy2, stroke, mark)
	// 对勾拐点加节点（隧道汇合点）
	fillCircle(out, bx, by, nodeR*1.15, mark)

	return out
}

func withAlpha(c color.RGBA, a uint8) color.RGBA {
	return color.RGBA{c.R, c.G, c.B, a}
}

func fillCircle(img *image.RGBA, cx, cy, r float64, c color.RGBA) {
	b := img.Bounds()
	ri := int(r) + 2
	for y := int(cy) - ri; y <= int(cy)+ri; y++ {
		for x := int(cx) - ri; x <= int(cx)+ri; x++ {
			if x < b.Min.X || y < b.Min.Y || x >= b.Max.X || y >= b.Max.Y {
				continue
			}
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			if d <= r {
				img.Set(x, y, c)
			} else if d < r+0.6 {
				t := r + 0.6 - d
				over := color.RGBA{c.R, c.G, c.B, uint8(float64(c.A) * t)}
				img.Set(x, y, blendOver(img.RGBAAt(x, y), over))
			}
		}
	}
}

func line(img *image.RGBA, x0, y0, x1, y1, width float64, c color.RGBA) {
	dx, dy := x1-x0, y1-y0
	length := math.Hypot(dx, dy)
	if length < 1e-6 {
		return
	}
	steps := int(length*2) + 1
	hw := width / 2
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := x0 + dx*t
		y := y0 + dy*t
		fillCircle(img, x, y, hw, c)
	}
}

func blendOver(dst, src color.RGBA) color.RGBA {
	if src.A == 255 {
		return src
	}
	if src.A == 0 {
		return dst
	}
	sa := float64(src.A) / 255
	da := float64(dst.A) / 255
	outA := sa + da*(1-sa)
	if outA < 1e-6 {
		return color.RGBA{}
	}
	return color.RGBA{
		R: uint8((float64(src.R)*sa + float64(dst.R)*da*(1-sa)) / outA),
		G: uint8((float64(src.G)*sa + float64(dst.G)*da*(1-sa)) / outA),
		B: uint8((float64(src.B)*sa + float64(dst.B)*da*(1-sa)) / outA),
		A: uint8(outA * 255),
	}
}

func resizeBilinear(src image.Image, size int) *image.RGBA {
	b := src.Bounds()
	sw, sh := float64(b.Dx()), float64(b.Dy())
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			sx := (float64(x)+0.5)*sw/float64(size) - 0.5 + float64(b.Min.X)
			sy := (float64(y)+0.5)*sh/float64(size) - 0.5 + float64(b.Min.Y)
			dst.Set(x, y, sampleBilinear(src, sx, sy))
		}
	}
	return dst
}

func sampleBilinear(src image.Image, x, y float64) color.Color {
	b := src.Bounds()
	x0 := int(math.Floor(x))
	y0 := int(math.Floor(y))
	x1, y1 := x0+1, y0+1
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 > b.Max.X-1 {
		x1 = b.Max.X - 1
	}
	if y1 > b.Max.Y-1 {
		y1 = b.Max.Y - 1
	}
	fx, fy := x-float64(x0), y-float64(y0)
	c00 := rgbaAt(src, x0, y0)
	c10 := rgbaAt(src, x1, y0)
	c01 := rgbaAt(src, x0, y1)
	c11 := rgbaAt(src, x1, y1)
	return color.RGBA{
		R: lerpByte(lerpByte(c00.R, c10.R, fx), lerpByte(c01.R, c11.R, fx), fy),
		G: lerpByte(lerpByte(c00.G, c10.G, fx), lerpByte(c01.G, c11.G, fx), fy),
		B: lerpByte(lerpByte(c00.B, c10.B, fx), lerpByte(c01.B, c11.B, fx), fy),
		A: lerpByte(lerpByte(c00.A, c10.A, fx), lerpByte(c01.A, c11.A, fx), fy),
	}
}

func rgbaAt(src image.Image, x, y int) color.RGBA {
	r, g, b, a := src.At(x, y).RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

func lerpByte(a, b uint8, t float64) uint8 {
	return uint8(float64(a)*(1-t) + float64(b)*t + 0.5)
}

func mustWritePNG(path string, img image.Image) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(err)
	}
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}
