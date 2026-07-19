package binarizer

import (
	"image"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// Otsu is the recommended default: an adaptive global threshold that maximizes
// between-class variance. Crisp edges (good for text/QR/barcode), single pass,
// negligible memory.
type Otsu struct{}

// NewOtsu returns the Otsu binarizer.
func NewOtsu() dp.Binarizer { return Otsu{} }

func (Otsu) Name() string { return "otsu" }

// Binarize thresholds at the Otsu level; pixels at or below the level are black.
func (Otsu) Binarize(img image.Image) *dp.MonoBitmap {
	g := toGray(img)
	t := otsuThreshold(g)
	return pack(g, func(v uint8) bool { return v <= t })
}

// otsuThreshold computes the optimal global threshold from the histogram.
func otsuThreshold(g *image.Gray) uint8 {
	var hist [256]int
	b := g.Bounds()
	total := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			hist[g.GrayAt(x, y).Y]++
			total++
		}
	}
	if total == 0 {
		return 128
	}

	var sum float64
	for i := 0; i < 256; i++ {
		sum += float64(i) * float64(hist[i])
	}

	var sumB float64
	wB := 0
	maxVar := -1.0
	// Track the plateau of maximum between-class variance and return its midpoint,
	// which is robust when several thresholds are equally optimal (e.g. a pure
	// black/white image) and reduces to the single argmax for real content.
	first, last := 128, 128
	for i := 0; i < 256; i++ {
		wB += hist[i]
		if wB == 0 {
			continue
		}
		wF := total - wB
		if wF == 0 {
			break
		}
		sumB += float64(i) * float64(hist[i])
		mB := sumB / float64(wB)
		mF := (sum - sumB) / float64(wF)
		between := float64(wB) * float64(wF) * (mB - mF) * (mB - mF)
		switch {
		case between > maxVar:
			maxVar = between
			first, last = i, i
		case between == maxVar:
			last = i
		}
	}
	return uint8((first + last) / 2)
}
