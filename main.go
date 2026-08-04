package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"os"
	"sort"
	"strconv"
	"strings"
)

var (
	outputFile  string
	numColors   int
	resizeSpec  string
	dedupFrames bool
)

func init() {
	flag.StringVar(&outputFile, "output", "output.gif", "output GIF file")
	flag.IntVar(&numColors, "colors", 256, "maximum palette colors (2-256)")
	flag.StringVar(&resizeSpec, "resize", "", "resize spec like 480x, x360 or 480x360")
	flag.BoolVar(&dedupFrames, "dedup", true, "collapse identical consecutive frames")
}

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: gifopt <input.gif> [flags]")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}
	inputFile := args[0]

	inStat, err := os.Stat(inputFile)
	if err != nil {
		fatalf("cannot stat input: %v", err)
	}
	inSize := inStat.Size()

	in, err := os.Open(inputFile)
	if err != nil {
		fatalf("cannot open input: %v", err)
	}
	g, err := gif.DecodeAll(in)
	in.Close()
	if err != nil {
		fatalf("decode failed: %v", err)
	}

	fmt.Printf("Input : %s (%d frames, %s)\n", inputFile, len(g.Image), humanSize(inSize))

	if resizeSpec != "" {
		tw, th, ok := parseResize(resizeSpec, g.Config.Width, g.Config.Height)
		if !ok {
			fatalf("invalid resize spec: %s", resizeSpec)
		}
		fmt.Printf("Resize: %dx%d -> %dx%d\n", g.Config.Width, g.Config.Height, tw, th)
		g = resizeGIF(g, tw, th)
	}

	if numColors < 2 {
		numColors = 2
	}
	if numColors > 256 {
		numColors = 256
	}
	if numColors < 256 {
		fmt.Printf("Colors: reducing palette to %d\n", numColors)
		g = reduceColors(g, numColors)
	}

	if dedupFrames && len(g.Image) > 1 {
		before := len(g.Image)
		g = deduplicate(g)
		fmt.Printf("Dedup : %d -> %d frames\n", before, len(g.Image))
	}

	out, err := os.Create(outputFile)
	if err != nil {
		fatalf("cannot create output: %v", err)
	}
	if err := gif.EncodeAll(out, g); err != nil {
		out.Close()
		fatalf("encode failed: %v", err)
	}
	out.Close()

	outStat, _ := os.Stat(outputFile)
	outSize := outStat.Size()
	saving := 0.0
	if inSize > 0 {
		saving = float64(inSize-outSize) / float64(inSize) * 100
	}

	fmt.Printf("Output: %s (%d frames, %s)\n", outputFile, len(g.Image), humanSize(outSize))
	fmt.Printf("Delta : %s -> %s (%.2f%% savings)\n", humanSize(inSize), humanSize(outSize), saving)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

func parseResize(spec string, origW, origH int) (int, int, bool) {
	parts := strings.Split(strings.ToLower(spec), "x")
	if len(parts) != 2 || (parts[0] == "" && parts[1] == "") {
		return 0, 0, false
	}
	var w, h int
	var err error
	if parts[0] == "" {
		h, err = strconv.Atoi(parts[1])
		if err != nil || h <= 0 {
			return 0, 0, false
		}
		w = origW * h / origH
	} else if parts[1] == "" {
		w, err = strconv.Atoi(parts[0])
		if err != nil || w <= 0 {
			return 0, 0, false
		}
		h = origH * w / origW
	} else {
		w, err = strconv.Atoi(parts[0])
		if err != nil || w <= 0 {
			return 0, 0, false
		}
		h, err = strconv.Atoi(parts[1])
		if err != nil || h <= 0 {
			return 0, 0, false
		}
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return w, h, true
}

func resizeGIF(g *gif.GIF, w, h int) *gif.GIF {
	origW := g.Config.Width
	origH := g.Config.Height
	if origW == 0 || origH == 0 {
		return g
	}
	newImages := make([]*image.Paletted, len(g.Image))
	for i, img := range g.Image {
		newImages[i] = resizePaletted(img, origW, origH, w, h)
	}
	g.Image = newImages
	g.Config.Width = w
	g.Config.Height = h
	return g
}

func resizePaletted(src *image.Paletted, origW, origH, targetW, targetH int) *image.Paletted {
	srcB := src.Bounds()
	minX := srcB.Min.X * targetW / origW
	minY := srcB.Min.Y * targetH / origH
	maxX := srcB.Max.X * targetW / origW
	maxY := srcB.Max.Y * targetH / origH
	if maxX <= minX {
		maxX = minX + 1
	}
	if maxY <= minY {
		maxY = minY + 1
	}
	dst := image.NewPaletted(image.Rect(minX, minY, maxX, maxY), src.Palette)
	dw := maxX - minX
	dh := maxY - minY
	sw := srcB.Dx()
	sh := srcB.Dy()
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			sx := srcB.Min.X + x*sw/dw
			sy := srcB.Min.Y + y*sh/dh
			dst.SetColorIndex(minX+x, minY+y, src.ColorIndexAt(sx, sy))
		}
	}
	return dst
}

type colorFreq struct {
	c color.RGBA
	n int
}

func reduceColors(g *gif.GIF, maxColors int) *gif.GIF {
	histogram := make(map[color.RGBA]int)
	for _, img := range g.Image {
		bounds := img.Bounds()
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, gr, bl, al := img.At(x, y).RGBA()
				key := color.RGBA{
					R: uint8(r >> 8),
					G: uint8(gr >> 8),
					B: uint8(bl >> 8),
					A: uint8(al >> 8),
				}
				histogram[key]++
			}
		}
	}

	freqs := make([]colorFreq, 0, len(histogram))
	for c, n := range histogram {
		freqs = append(freqs, colorFreq{c, n})
	}
	sort.Slice(freqs, func(i, j int) bool { return freqs[i].n > freqs[j].n })

	limit := maxColors
	if limit > len(freqs) {
		limit = len(freqs)
	}
	if limit < 1 {
		limit = 1
	}

	newPalette := make(color.Palette, limit)
	for i := 0; i < limit; i++ {
		newPalette[i] = freqs[i].c
	}

	newImages := make([]*image.Paletted, len(g.Image))
	for i, img := range g.Image {
		newImg := image.NewPaletted(img.Bounds(), newPalette)
		draw.FloydSteinberg.Draw(newImg, img.Bounds(), img, img.Bounds().Min)
		newImages[i] = newImg
	}
	g.Image = newImages
	return g
}

func deduplicate(g *gif.GIF) *gif.GIF {
	if len(g.Image) < 2 {
		return g
	}
	hasDisposal := len(g.Disposal) == len(g.Image)

	newImages := []*image.Paletted{g.Image[0]}
	newDelays := []int{g.Delay[0]}
	var newDisposal []byte
	if hasDisposal {
		newDisposal = []byte{g.Disposal[0]}
	}

	for i := 1; i < len(g.Image); i++ {
		last := newImages[len(newImages)-1]
		if framesEqual(g.Image[i], last) {
			newDelays[len(newDelays)-1] += g.Delay[i]
			continue
		}
		newImages = append(newImages, g.Image[i])
		newDelays = append(newDelays, g.Delay[i])
		if hasDisposal {
			newDisposal = append(newDisposal, g.Disposal[i])
		}
	}
	g.Image = newImages
	g.Delay = newDelays
	if hasDisposal {
		g.Disposal = newDisposal
	}
	return g
}

func framesEqual(a, b *image.Paletted) bool {
	if !a.Bounds().Eq(b.Bounds()) {
		return false
	}
	if len(a.Palette) == len(b.Palette) && bytes.Equal(a.Pix, b.Pix) {
		same := true
		for i, ca := range a.Palette {
			cb := b.Palette[i]
			r1, g1, b1, a1 := ca.RGBA()
			r2, g2, b2, a2 := cb.RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				same = false
				break
			}
		}
		if same {
			return true
		}
	}
	bounds := a.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r1, g1, b1, a1 := a.At(x, y).RGBA()
			r2, g2, b2, a2 := b.At(x, y).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				return false
			}
		}
	}
	return true
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
