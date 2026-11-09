//go:build ignore

// Generates the installable-app icons in webui/public from docs/brand/app-icon-1024.png.
// One-off, run by hand from the repo root: go run scripts/gen-pwa-icons.go
// The PNGs are committed, so the build needs no image tooling.
package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
)

const (
	source = "docs/brand/app-icon-1024.png"
	outDir = "webui/public/"
	// maskableArt is the share of the canvas the art fills; the rest is the
	// source's navy, which a launcher's mask may crop.
	maskableArt = 0.70
)

func main() {
	f, err := os.Open(source)
	check(err)
	src, err := png.Decode(f)
	check(err)
	check(f.Close())

	write("pwa-icon-192.png", scale(src, 192))
	write("pwa-icon-512.png", scale(src, 512))

	size := 512
	art := int(float64(size) * maskableArt)
	canvas := image.NewRGBA(image.Rect(0, 0, size, size))
	// The source is a full-bleed square, so its corner is the navy.
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{color.RGBAModel.Convert(src.At(0, 0))}, image.Point{}, draw.Src)
	off := (size - art) / 2
	draw.Draw(canvas, image.Rect(off, off, off+art, off+art), scale(src, art), image.Point{}, draw.Over)
	write("pwa-icon-maskable-512.png", canvas)
}

// scale shrinks src to size x size by averaging each source box.
func scale(src image.Image, size int) *image.RGBA {
	b := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		y0, y1 := b.Min.Y+y*b.Dy()/size, b.Min.Y+(y+1)*b.Dy()/size
		for x := 0; x < size; x++ {
			x0, x1 := b.Min.X+x*b.Dx()/size, b.Min.X+(x+1)*b.Dx()/size
			var r, g, bl, a, n uint32
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					pr, pg, pb, pa := src.At(sx, sy).RGBA()
					r, g, bl, a, n = r+pr, g+pg, bl+pb, a+pa, n+1
				}
			}
			out.SetRGBA(x, y, color.RGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(bl / n >> 8), uint8(a / n >> 8)})
		}
	}
	return out
}

func write(name string, img image.Image) {
	f, err := os.Create(outDir + name)
	check(err)
	check(png.Encode(f, img))
	check(f.Close())
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
