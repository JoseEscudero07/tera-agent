package binarizer

import (
	"image"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// Threshold is a fixed-level binarizer. Fast and predictable; use Otsu when the
// optimal level is unknown.
type Threshold struct{ Level uint8 }

// NewThreshold returns a fixed-threshold binarizer (typical level: 128).
func NewThreshold(level uint8) dp.Binarizer { return Threshold{Level: level} }

func (Threshold) Name() string { return "threshold" }

func (t Threshold) Binarize(img image.Image) *dp.MonoBitmap {
	g := toGray(img)
	return pack(g, func(v uint8) bool { return v < t.Level })
}
