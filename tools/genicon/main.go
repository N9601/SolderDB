// genicon paints the SolderDB app icon and writes both:
//
//	build/appicon.png       , 1024x1024 PNG for Wails / macOS bundles
//	build/windows/icon.ico  , multi-resolution Windows .ico file
//
// Stdlib only. Run from the repo root with:
//
//	go run ./tools/genicon
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// Brand palette, kept in sync with frontend/tailwind.config.ts.
var (
	bgInk      = color.NRGBA{R: 0x14, G: 0x18, B: 0x1f, A: 0xff}
	bgInkLight = color.NRGBA{R: 0x25, G: 0x2b, B: 0x35, A: 0xff}
	steel      = color.NRGBA{R: 0x5b, G: 0x6b, B: 0x8a, A: 0xff}
	steelHi    = color.NRGBA{R: 0xa9, G: 0xb6, B: 0xc8, A: 0xff}
	copper     = color.NRGBA{R: 0xe0, G: 0x7a, B: 0x25, A: 0xff}
	copperHi   = color.NRGBA{R: 0xf4, G: 0xb8, B: 0x7a, A: 0xff}
	sparkCore  = color.NRGBA{R: 0xff, G: 0xf3, B: 0xd6, A: 0xff}
)

const (
	masterSize = 1024
)

func main() {
	// Resolve paths relative to wherever the command was run from.
	if err := os.MkdirAll("build/windows", 0o755); err != nil {
		die("mkdir build/windows: %v", err)
	}

	img := renderIcon(masterSize)

	if err := writePNG(filepath.Join("build", "appicon.png"), img); err != nil {
		die("write appicon.png: %v", err)
	}
	fmt.Println("✓ build/appicon.png")

	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	imgs := make([]*image.NRGBA, 0, len(sizes))
	for _, s := range sizes {
		imgs = append(imgs, downsample(img, s))
	}
	if err := writeICO(filepath.Join("build", "windows", "icon.ico"), imgs); err != nil {
		die("write icon.ico: %v", err)
	}
	fmt.Printf("✓ build/windows/icon.ico (%d sizes)\n", len(sizes))
	fmt.Println("\nNext: re-run `wails dev` (or `wails build`) to bake the new icon in.")
}

// ---------------- Drawing ----------------

func renderIcon(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	cx, cy := float64(size)/2, float64(size)/2
	pad := float64(size) * 0.06
	r := float64(size)/2 - pad

	// Rounded-square background with vertical gradient.
	drawRoundedRect(img, int(pad), int(pad), size-int(pad), size-int(pad), int(float64(size)*0.18), func(_, y int) color.NRGBA {
		t := (float64(y) - pad) / (float64(size) - 2*pad)
		return mix(bgInkLight, bgInk, clamp(t, 0, 1))
	})

	// Three stacked database disks, slightly offset, growing brighter toward the top.
	diskW := r * 1.10
	diskH := diskW * 0.30
	gap := diskH * 0.55
	totalH := diskH + 2*gap + 3*diskH*0.4
	startY := cy - totalH/2 + diskH/2

	for i := 0; i < 3; i++ {
		yy := startY + float64(i)*(gap+diskH*0.5)
		baseShade := steel
		if i == 0 {
			baseShade = steelHi
		}
		drawDisk(img, cx, yy, diskW/2, diskH/2, baseShade, steel)
	}

	// Connection nodes, small steel dots flanking the cylinder hinting at a network.
	nodeR := r * 0.05
	fillCircle(img, cx-r*0.85, cy+r*0.05, nodeR, steel, 1)
	fillCircle(img, cx+r*0.85, cy+r*0.05, nodeR, steel, 1)
	fillCircle(img, cx-r*0.85, cy+r*0.35, nodeR*0.7, steel, 1)
	fillCircle(img, cx+r*0.85, cy+r*0.35, nodeR*0.7, steel, 1)

	// Spark at the top of the cylinder, copper glow + bright core.
	sparkX := cx + r*0.20
	sparkY := cy - r*0.18
	drawSparkGlow(img, sparkX, sparkY, r*0.42)
	fillCircle(img, sparkX, sparkY, r*0.10, copperHi, 1)
	fillCircle(img, sparkX, sparkY, r*0.05, sparkCore, 1)

	return img
}

// drawDisk paints an ellipse at (cx,cy) with horizontal+vertical radii.
// `top` is the brighter shade for the upper half, `bottom` for the lower.
func drawDisk(img *image.NRGBA, cx, cy, rx, ry float64, top, bottom color.NRGBA) {
	bounds := img.Bounds()
	x0 := int(math.Floor(cx - rx - 1))
	x1 := int(math.Ceil(cx + rx + 1))
	y0 := int(math.Floor(cy - ry - 1))
	y1 := int(math.Ceil(cy + ry + 1))
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > bounds.Max.X {
		x1 = bounds.Max.X
	}
	if y1 > bounds.Max.Y {
		y1 = bounds.Max.Y
	}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			dx := (float64(x) + 0.5 - cx) / rx
			dy := (float64(y) + 0.5 - cy) / ry
			d := dx*dx + dy*dy
			if d > 1 {
				continue
			}
			alpha := 1.0
			if d > 0.9 {
				alpha = 1.0 - (d-0.9)*10 // soft edge
			}
			t := (float64(y) - (cy - ry)) / (2 * ry)
			c := mix(top, bottom, clamp(t, 0, 1))
			c.A = uint8(float64(c.A) * alpha)
			blend(img, x, y, c)
		}
	}
}

// drawSparkGlow paints a smooth radial copper glow centered at (cx,cy)
// with the given outer radius.
func drawSparkGlow(img *image.NRGBA, cx, cy, r float64) {
	bounds := img.Bounds()
	x0 := int(math.Floor(cx - r - 1))
	x1 := int(math.Ceil(cx + r + 1))
	y0 := int(math.Floor(cy - r - 1))
	y1 := int(math.Ceil(cy + r + 1))
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > bounds.Max.X {
		x1 = bounds.Max.X
	}
	if y1 > bounds.Max.Y {
		y1 = bounds.Max.Y
	}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			dx := float64(x) + 0.5 - cx
			dy := float64(y) + 0.5 - cy
			d := math.Sqrt(dx*dx+dy*dy) / r
			if d > 1 {
				continue
			}
			// Two-stop falloff: bright copper core, soft outer.
			t := d
			var glow color.NRGBA
			if t < 0.5 {
				glow = mix(copperHi, copper, t*2)
				glow.A = uint8(float64(glow.A) * (1 - t*0.4))
			} else {
				glow = copper
				alpha := math.Pow(1.0-(t-0.5)*2, 1.8)
				glow.A = uint8(float64(glow.A) * alpha)
			}
			blend(img, x, y, glow)
		}
	}
}

// drawRoundedRect fills a rounded rect, calling `colorAt` per pixel so
// callers can paint gradients.
func drawRoundedRect(img *image.NRGBA, x0, y0, x1, y1, radius int, colorAt func(x, y int) color.NRGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			// Distance to nearest corner, if inside the inner rect, draw.
			dx, dy := 0, 0
			if x < x0+radius {
				dx = (x0 + radius) - x
			} else if x >= x1-radius {
				dx = x - (x1 - radius - 1)
			}
			if y < y0+radius {
				dy = (y0 + radius) - y
			} else if y >= y1-radius {
				dy = y - (y1 - radius - 1)
			}
			if dx == 0 || dy == 0 {
				blend(img, x, y, colorAt(x, y))
				continue
			}
			d := math.Sqrt(float64(dx*dx + dy*dy))
			if d > float64(radius)+0.5 {
				continue
			}
			alpha := 1.0
			if d > float64(radius)-0.5 {
				alpha = float64(radius) + 0.5 - d
			}
			c := colorAt(x, y)
			c.A = uint8(float64(c.A) * alpha)
			blend(img, x, y, c)
		}
	}
}

func fillCircle(img *image.NRGBA, cx, cy, r float64, col color.NRGBA, opacity float64) {
	bounds := img.Bounds()
	x0 := int(math.Floor(cx - r - 1))
	x1 := int(math.Ceil(cx + r + 1))
	y0 := int(math.Floor(cy - r - 1))
	y1 := int(math.Ceil(cy + r + 1))
	if x0 < bounds.Min.X {
		x0 = bounds.Min.X
	}
	if y0 < bounds.Min.Y {
		y0 = bounds.Min.Y
	}
	if x1 > bounds.Max.X {
		x1 = bounds.Max.X
	}
	if y1 > bounds.Max.Y {
		y1 = bounds.Max.Y
	}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			dx := float64(x) + 0.5 - cx
			dy := float64(y) + 0.5 - cy
			d := math.Sqrt(dx*dx + dy*dy)
			if d > r+0.5 {
				continue
			}
			alpha := opacity
			if d > r-0.5 {
				alpha *= (r + 0.5 - d)
			}
			c := col
			c.A = uint8(float64(c.A) * alpha)
			blend(img, x, y, c)
		}
	}
}

// ---------------- Color helpers ----------------

func mix(a, b color.NRGBA, t float64) color.NRGBA {
	t = clamp(t, 0, 1)
	return color.NRGBA{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t),
		A: uint8(float64(a.A)*(1-t) + float64(b.A)*t),
	}
}

func blend(img *image.NRGBA, x, y int, src color.NRGBA) {
	if src.A == 0 {
		return
	}
	idx := img.PixOffset(x, y)
	dr := float64(img.Pix[idx+0])
	dg := float64(img.Pix[idx+1])
	db := float64(img.Pix[idx+2])
	da := float64(img.Pix[idx+3])
	sa := float64(src.A) / 255
	outA := sa + da/255*(1-sa)
	if outA == 0 {
		return
	}
	img.Pix[idx+0] = uint8((float64(src.R)*sa + dr/255*da*(1-sa)) / outA)
	img.Pix[idx+1] = uint8((float64(src.G)*sa + dg/255*da*(1-sa)) / outA)
	img.Pix[idx+2] = uint8((float64(src.B)*sa + db/255*da*(1-sa)) / outA)
	img.Pix[idx+3] = uint8(outA * 255)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ---------------- Resize ----------------

// downsample produces a smaller image using a simple area-average filter.
// Fast, no bias, gives clean icon-size results on flat brand artwork.
func downsample(src *image.NRGBA, target int) *image.NRGBA {
	srcW := src.Bounds().Dx()
	srcH := src.Bounds().Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, target, target))
	if target == srcW {
		copy(dst.Pix, src.Pix)
		return dst
	}
	scaleX := float64(srcW) / float64(target)
	scaleY := float64(srcH) / float64(target)
	for dy := 0; dy < target; dy++ {
		y0 := int(float64(dy) * scaleY)
		y1 := int(float64(dy+1) * scaleY)
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for dx := 0; dx < target; dx++ {
			x0 := int(float64(dx) * scaleX)
			x1 := int(float64(dx+1) * scaleX)
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, b, a, n float64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					i := src.PixOffset(xx, yy)
					r += float64(src.Pix[i+0])
					g += float64(src.Pix[i+1])
					b += float64(src.Pix[i+2])
					a += float64(src.Pix[i+3])
					n++
				}
			}
			j := dst.PixOffset(dx, dy)
			dst.Pix[j+0] = uint8(r / n)
			dst.Pix[j+1] = uint8(g / n)
			dst.Pix[j+2] = uint8(b / n)
			dst.Pix[j+3] = uint8(a / n)
		}
	}
	return dst
}

// ---------------- File output ----------------

func writePNG(path string, img *image.NRGBA) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return png.Encode(f, img)
}

// writeICO bundles several PNG-encoded images into a Windows .ico container.
// Modern ICO entries embed full PNGs, which is simpler than BMP-style entries
// and is supported by Windows Vista+.
func writeICO(path string, imgs []*image.NRGBA) error {
	type entry struct {
		w, h int
		png  []byte
	}
	entries := make([]entry, 0, len(imgs))
	for _, im := range imgs {
		var buf bytes.Buffer
		if err := png.Encode(&buf, im); err != nil {
			return err
		}
		entries = append(entries, entry{w: im.Bounds().Dx(), h: im.Bounds().Dy(), png: buf.Bytes()})
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	// ICONDIR
	if err := binary.Write(out, binary.LittleEndian, uint16(0)); err != nil { // Reserved
		return err
	}
	if err := binary.Write(out, binary.LittleEndian, uint16(1)); err != nil { // Type = 1 (icon)
		return err
	}
	if err := binary.Write(out, binary.LittleEndian, uint16(len(entries))); err != nil {
		return err
	}

	// Calculate where each image's bytes will start.
	headerSize := 6 + 16*len(entries)
	offsets := make([]uint32, len(entries))
	off := uint32(headerSize)
	for i, e := range entries {
		offsets[i] = off
		off += uint32(len(e.png))
	}

	// ICONDIRENTRYs
	for i, e := range entries {
		dim := func(v int) byte {
			if v >= 256 {
				return 0
			}
			return byte(v)
		}
		_, _ = out.Write([]byte{
			dim(e.w),
			dim(e.h),
			0, // color count (0 for >=256)
			0, // reserved
		})
		if err := binary.Write(out, binary.LittleEndian, uint16(1)); err != nil { // planes
			return err
		}
		if err := binary.Write(out, binary.LittleEndian, uint16(32)); err != nil { // bpp
			return err
		}
		if err := binary.Write(out, binary.LittleEndian, uint32(len(e.png))); err != nil {
			return err
		}
		if err := binary.Write(out, binary.LittleEndian, offsets[i]); err != nil {
			return err
		}
	}

	// PNG payloads
	for _, e := range entries {
		if _, err := out.Write(e.png); err != nil {
			return err
		}
	}
	return nil
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "genicon: "+format+"\n", args...)
	os.Exit(1)
}
