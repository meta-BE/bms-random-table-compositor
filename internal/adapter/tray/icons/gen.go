//go:build ignore

// Package main は tray アイコン PNG を生成するワンショットスクリプト。
// Usage: go run gen.go
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

type spec struct {
	name string
	c    color.NRGBA
}

func main() {
	dir, err := os.Getwd()
	must(err)
	specs := []spec{
		{"idle.png", color.NRGBA{R: 128, G: 128, B: 128, A: 255}},
		{"running.png", color.NRGBA{R: 56, G: 161, B: 105, A: 255}},
		{"error.png", color.NRGBA{R: 198, G: 56, B: 56, A: 255}},
	}
	for _, s := range specs {
		img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
		for y := 0; y < 16; y++ {
			for x := 0; x < 16; x++ {
				img.Set(x, y, s.c)
			}
		}
		f, err := os.Create(filepath.Join(dir, s.name))
		must(err)
		must(png.Encode(f, img))
		_ = f.Close()
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
