package main

import (
	"bytes"
	"compress/zlib"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"strings"
)

// shortcut: previews decode PNG, JPEG, and GIF files up to 8 MiB and 20 MP; add decoders or raise limits when other images are needed.
const maxPreviewBytes = 8 << 20

func renderImagePreview(src image.Image, cols, rows int) (string, string, bool, error) {
	bounds := src.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 || int64(bounds.Dx())*int64(bounds.Dy()) > 20_000_000 {
		return "", "", false, fmt.Errorf("image dimensions exceed the preview limit")
	}
	cols = max(1, cols)
	rows = max(1, rows)
	if kittyGraphicsAvailable() {
		pixels, width, height := fitRGBA(src, cols*8, rows*16)
		kitty, clear, err := kittyImage(pixels, width, height, cols, rows)
		return kitty, clear, true, err
	}
	return halfBlockImage(src, cols, rows), "", false, nil
}

func kittyGraphicsAvailable() bool {
	if os.Getenv("TMUX") != "" || os.Getenv("STY") != "" {
		return false
	}
	if os.Getenv("KITTY_WINDOW_ID") != "" || strings.Contains(os.Getenv("TERM"), "xterm-kitty") {
		return true
	}
	switch strings.ToLower(os.Getenv("TERM_PROGRAM")) {
	case "kitty", "ghostty", "wezterm":
		return true
	default:
		return false
	}
}

func fitRGBA(src image.Image, maxWidth, maxHeight int) ([]byte, int, int) {
	b := src.Bounds()
	scale := math.Min(float64(maxWidth)/float64(b.Dx()), float64(maxHeight)/float64(b.Dy()))
	w := max(1, int(math.Round(float64(b.Dx())*scale)))
	h := max(1, int(math.Round(float64(b.Dy())*scale)))
	pixels := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		sy := float64(b.Min.Y) + (float64(y)+0.5)*float64(b.Dy())/float64(h) - 0.5
		for x := 0; x < w; x++ {
			sx := float64(b.Min.X) + (float64(x)+0.5)*float64(b.Dx())/float64(w) - 0.5
			c := sampleBilinear(src, sx, sy)
			i := (y*w + x) * 4
			pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = c.R, c.G, c.B, c.A
		}
	}
	return pixels, w, h
}

func sampleBilinear(src image.Image, x, y float64) color.NRGBA {
	b := src.Bounds()
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	dx, dy := x-float64(x0), y-float64(y0)
	blend := func(a, b color.NRGBA, t float64) color.NRGBA {
		mix := func(x, y uint8) uint8 { return uint8(math.Round(float64(x)*(1-t) + float64(y)*t)) }
		return color.NRGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: mix(a.A, b.A)}
	}
	point := func(x, y int) color.NRGBA {
		return color.NRGBAModel.Convert(src.At(max(b.Min.X, min(x, b.Max.X-1)), max(b.Min.Y, min(y, b.Max.Y-1)))).(color.NRGBA)
	}
	return blend(blend(point(x0, y0), point(x0+1, y0), dx), blend(point(x0, y0+1), point(x0+1, y0+1), dx), dy)
}

func kittyImage(pixels []byte, width, height, cols, rows int) (string, string, error) {
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(pixels); err != nil {
		return "", "", err
	}
	if err := zw.Close(); err != nil {
		return "", "", err
	}
	encoded := base64.StdEncoding.EncodeToString(compressed.Bytes())
	var randomID [4]byte
	if _, err := rand.Read(randomID[:]); err != nil {
		return "", "", err
	}
	id := uint32(randomID[0])<<24 | uint32(randomID[1])<<16 | uint32(randomID[2])<<8 | uint32(randomID[3])
	id &= 0x7fffffff
	if id == 0 {
		id = 1
	}
	var out strings.Builder
	for offset, first := 0, true; offset < len(encoded); first = false {
		end := min(offset+4096, len(encoded))
		more := 0
		if end < len(encoded) {
			more = 1
		}
		if first {
			fmt.Fprintf(&out, "\x1b_Ga=T,f=32,o=z,t=d,s=%d,v=%d,c=%d,r=%d,C=1,i=%d,q=2,m=%d;%s\x1b\\", width, height, cols, rows, id, more, encoded[offset:end])
		} else {
			fmt.Fprintf(&out, "\x1b_Gm=%d,q=2;%s\x1b\\", more, encoded[offset:end])
		}
		offset = end
	}
	for row := 0; row < rows; row++ {
		if row > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(strings.Repeat(" ", cols))
	}
	clear := fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id)
	return out.String(), clear, nil
}

func halfBlockImage(src image.Image, cols, rows int) string {
	pixels, width, height := fitRGBA(src, cols, rows*2)
	var out strings.Builder
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			top := pixelAt(pixels, width, height, col, row*2)
			bottom := pixelAt(pixels, width, height, col, row*2+1)
			fmt.Fprintf(&out, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm▀\x1b[0m", top.R, top.G, top.B, bottom.R, bottom.G, bottom.B)
		}
		if row+1 < rows {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

func pixelAt(pixels []byte, width, height, x, y int) color.NRGBA {
	if x >= width || y >= height {
		return color.NRGBA{}
	}
	i := (y*width + x) * 4
	return color.NRGBA{R: pixels[i], G: pixels[i+1], B: pixels[i+2], A: pixels[i+3]}
}
