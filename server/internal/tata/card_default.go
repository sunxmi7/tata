package tata

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

const (
	defaultCardThumbnailName = "card-default.png"
	defaultCardThumbnailPath = "/uploads/card-default.png"
	thumbnailDefaultKey      = "card.thumbnail_default"
)

var defaultCardPNG = mustDefaultCardPNG()

func mustDefaultCardPNG() []byte {
	const w, h = 96, 160
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	navy := color.RGBA{0x1A, 0x13, 0x33, 0xFF}
	gold := color.RGBA{0xC9, 0xA8, 0x6C, 0xFF}
	inner := color.RGBA{0x2A, 0x1F, 0x4D, 0xFF}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := navy
			edge := x < 4 || y < 4 || x >= w-4 || y >= h-4
			frame := x < 10 || y < 10 || x >= w-10 || y >= h-10
			dx, dy := x-w/2, y-h/2
			if dx < 0 {
				dx = -dx
			}
			if dy < 0 {
				dy = -dy
			}
			diamond := dx+dy*3/5 < 18
			switch {
			case edge:
				c = gold
			case frame:
				c = inner
			case diamond:
				c = gold
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
