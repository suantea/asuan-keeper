// Package tray 提供 asuan 系统托盘（Windows/macOS 共用）。
// 单击图标→查看同步进度，双击→打开配置界面，右键→菜单（退出/暂停-同步/打开控制台）。
// 底层使用 third_party/systray（fork 自 getlantern/systray，扩展了单击/双击回调）。
package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
)

const iconSize = 32

// Icon 生成托盘图标：返回 (icoBytes, pngBytes, err)。
// Windows 用 ICO（PNG 压缩，Vista+ 支持）；macOS/Linux 用 PNG。
func Icon() ([]byte, []byte, error) {
	img := buildIcon()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, nil, err
	}
	pngBytes := buf.Bytes()
	return wrapICO(pngBytes), pngBytes, nil
}

// buildIcon 绘制 32x32 图标：蓝色圆底 + 白色同步环（带缺口箭头）。
func buildIcon() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.Transparent}, image.Point{}, draw.Src)
	cx, cy := 15.5, 15.5
	blue := color.RGBA{R: 0x24, G: 0x6e, B: 0xff, A: 0xff}
	white := color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	for y := 0; y < iconSize; y++ {
		for x := 0; x < iconSize; x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			d := math.Hypot(dx, dy)
			deg := math.Atan2(dy, dx) * 180 / math.Pi
			if deg < 0 {
				deg += 360
			}
			inGap := deg >= 40 && deg <= 80 && dx >= 0 // 同步环缺口（右下→右上）
			switch {
			case d <= 13.2:
				img.SetRGBA(x, y, blue)
			case d >= 9.0 && d <= 11.0 && !inGap:
				img.SetRGBA(x, y, white)
			case inGap && d >= 8.0 && d <= 11.5:
				img.SetRGBA(x, y, white) // 缺口处箭头
			}
		}
	}
	return img
}

// wrapICO 把 PNG 数据打包成单条目 ICO（ICONDIR + ICONDIRENTRY + PNG）。
// Windows Vista+ 支持 PNG 压缩 ICO；宽度/高度为 0 表示 256，这里固定 32。
func wrapICO(pngBytes []byte) []byte {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, uint16(0)) // reserved
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // type=icon
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // count
	buf.WriteByte(iconSize)                                // width
	buf.WriteByte(iconSize)                                // height
	buf.WriteByte(0)                                       // color count
	buf.WriteByte(0)                                       // reserved
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // planes
	_ = binary.Write(&buf, binary.LittleEndian, uint16(32)) // bit count
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(pngBytes)))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(6+16)) // image offset
	buf.Write(pngBytes)
	return buf.Bytes()
}
