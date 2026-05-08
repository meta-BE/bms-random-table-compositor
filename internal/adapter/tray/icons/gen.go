//go:build ignore

// Package main は tray アイコン PNG / ICO を生成するワンショットスクリプト。
// Usage: go run gen.go
//
// Linux / macOS は PNG、Windows は ICO 形式が必要 (fyne.io/systray の制約)。
// ICO ファイルは PNG を内部に埋め込む形式 (Vista 以降サポート) で生成する。
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

type spec struct {
	base string
	c    color.NRGBA
}

func main() {
	dir, err := os.Getwd()
	must(err)
	specs := []spec{
		{"idle", color.NRGBA{R: 128, G: 128, B: 128, A: 255}},
		{"running", color.NRGBA{R: 56, G: 161, B: 105, A: 255}},
		{"error", color.NRGBA{R: 198, G: 56, B: 56, A: 255}},
	}
	for _, s := range specs {
		// 16x16 純色画像を生成
		img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
		for y := 0; y < 16; y++ {
			for x := 0; x < 16; x++ {
				img.Set(x, y, s.c)
			}
		}

		// PNG (Linux / 開発時参照用)
		var pngBuf bytes.Buffer
		must(png.Encode(&pngBuf, img))
		must(os.WriteFile(filepath.Join(dir, s.base+".png"), pngBuf.Bytes(), 0644))

		// ICO (Windows 用)。ICONDIR + ICONDIRENTRY + 埋め込み PNG。
		pngBytes := pngBuf.Bytes()
		var ico bytes.Buffer
		// ICONDIR (6 bytes)
		must(binary.Write(&ico, binary.LittleEndian, uint16(0)))               // Reserved
		must(binary.Write(&ico, binary.LittleEndian, uint16(1)))               // Type = ICO
		must(binary.Write(&ico, binary.LittleEndian, uint16(1)))               // Count = 1 image
		// ICONDIRENTRY (16 bytes)
		ico.WriteByte(16)                                                       // Width
		ico.WriteByte(16)                                                       // Height
		ico.WriteByte(0)                                                        // ColorCount (0=true color)
		ico.WriteByte(0)                                                        // Reserved
		must(binary.Write(&ico, binary.LittleEndian, uint16(1)))               // Planes
		must(binary.Write(&ico, binary.LittleEndian, uint16(32)))              // BitCount
		must(binary.Write(&ico, binary.LittleEndian, uint32(len(pngBytes))))   // BytesInRes
		must(binary.Write(&ico, binary.LittleEndian, uint32(22)))              // ImageOffset (6+16)
		// PNG bytes
		ico.Write(pngBytes)
		must(os.WriteFile(filepath.Join(dir, s.base+".ico"), ico.Bytes(), 0644))
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
