// Command icon renders the deliberately simple, classic IMAJER application icon.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: icon OUTPUT.png|OUTPUT.icns")
		os.Exit(2)
	}
	canvas := render()
	var err error
	if strings.EqualFold(filepath.Ext(os.Args[1]), ".icns") {
		err = writeICNS(os.Args[1], canvas)
	} else {
		err = writePNG(os.Args[1], canvas)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func render() *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
	fill(canvas, image.Rect(0, 0, 1024, 1024), color.RGBA{0, 0, 0, 0})

	shadow := image.Rect(124, 140, 924, 928)
	fill(canvas, shadow, color.RGBA{70, 70, 70, 255})
	window := image.Rect(92, 96, 892, 884)
	fill(canvas, window, color.RGBA{212, 208, 200, 255})
	frame(canvas, window, 20, color.RGBA{255, 255, 255, 255}, color.RGBA{64, 64, 64, 255})

	title := image.Rect(132, 136, 852, 296)
	fill(canvas, title, color.RGBA{0, 0, 128, 255})
	closeBox := image.Rect(700, 168, 812, 264)
	fill(canvas, closeBox, color.RGBA{212, 208, 200, 255})
	frame(canvas, closeBox, 10, color.RGBA{255, 255, 255, 255}, color.RGBA{64, 64, 64, 255})
	fill(canvas, image.Rect(728, 204, 784, 220), color.RGBA{0, 0, 0, 255})

	panel := image.Rect(172, 344, 812, 776)
	fill(canvas, panel, color.RGBA{255, 255, 255, 255})
	frame(canvas, panel, 14, color.RGBA{96, 96, 96, 255}, color.RGBA{255, 255, 255, 255})
	drawLetters(canvas)

	button := image.Rect(312, 800, 672, 852)
	fill(canvas, button, color.RGBA{212, 208, 200, 255})
	frame(canvas, button, 8, color.RGBA{255, 255, 255, 255}, color.RGBA{64, 64, 64, 255})
	fill(canvas, image.Rect(392, 818, 592, 834), color.RGBA{0, 0, 0, 255})
	return canvas
}

func writePNG(path string, canvas image.Image) error {
	output, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer output.Close()
	return png.Encode(output, canvas)
}

func writeICNS(path string, master image.Image) error {
	entries := []struct {
		kind string
		size int
	}{
		{"icp4", 16},
		{"icp5", 32},
		{"icp6", 64},
		{"ic07", 128},
		{"ic08", 256},
		{"ic09", 512},
		{"ic10", 1024},
	}
	var body bytes.Buffer
	for _, entry := range entries {
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, resizeNearest(master, entry.size)); err != nil {
			return err
		}
		body.WriteString(entry.kind)
		if err := binary.Write(&body, binary.BigEndian, uint32(encoded.Len()+8)); err != nil {
			return err
		}
		body.Write(encoded.Bytes())
	}
	var result bytes.Buffer
	result.WriteString("icns")
	if err := binary.Write(&result, binary.BigEndian, uint32(body.Len()+8)); err != nil {
		return err
	}
	result.Write(body.Bytes())
	return os.WriteFile(path, result.Bytes(), 0o644)
}

func resizeNearest(source image.Image, size int) *image.RGBA {
	target := image.NewRGBA(image.Rect(0, 0, size, size))
	bounds := source.Bounds()
	for y := range size {
		sourceY := bounds.Min.Y + y*bounds.Dy()/size
		for x := range size {
			sourceX := bounds.Min.X + x*bounds.Dx()/size
			target.Set(x, y, source.At(sourceX, sourceY))
		}
	}
	return target
}

func drawLetters(canvas *image.RGBA) {
	ink := color.RGBA{0, 0, 128, 255}
	fill(canvas, image.Rect(248, 432, 312, 688), ink)
	fill(canvas, image.Rect(376, 432, 440, 688), ink)
	fill(canvas, image.Rect(440, 432, 504, 512), ink)
	fill(canvas, image.Rect(504, 496, 568, 576), ink)
	fill(canvas, image.Rect(568, 432, 632, 512), ink)
	fill(canvas, image.Rect(632, 432, 696, 688), ink)
}

func frame(canvas *image.RGBA, rectangle image.Rectangle, width int, topLeft, bottomRight color.Color) {
	fill(canvas, image.Rect(rectangle.Min.X, rectangle.Min.Y, rectangle.Max.X, rectangle.Min.Y+width), topLeft)
	fill(canvas, image.Rect(rectangle.Min.X, rectangle.Min.Y, rectangle.Min.X+width, rectangle.Max.Y), topLeft)
	fill(canvas, image.Rect(rectangle.Min.X, rectangle.Max.Y-width, rectangle.Max.X, rectangle.Max.Y), bottomRight)
	fill(canvas, image.Rect(rectangle.Max.X-width, rectangle.Min.Y, rectangle.Max.X, rectangle.Max.Y), bottomRight)
}

func fill(canvas *image.RGBA, rectangle image.Rectangle, shade color.Color) {
	draw.Draw(canvas, rectangle, &image.Uniform{C: shade}, image.Point{}, draw.Src)
}
