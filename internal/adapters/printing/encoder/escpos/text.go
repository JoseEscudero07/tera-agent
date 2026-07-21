package escpos

import (
	"bytes"
	"context"
	"fmt"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// Text encodes a text artifact as ESC/POS (init + body + feed + optional cut).
type Text struct{}

// NewText returns an ESC/POS text encoder.
func NewText() dp.Encoder { return Text{} }

func (Text) Accepts() dp.ArtifactKind  { return dp.ArtifactText }
func (Text) Produces() dp.DeviceFormat { return dp.DeviceESCPOS }

func (Text) Encode(_ context.Context, a dp.Artifact, opts dp.EncodeOptions) ([]byte, error) {
	ta, ok := a.(dp.TextArtifact)
	if !ok {
		return nil, fmt.Errorf("escpos text: expected text artifact, got %s", a.Kind())
	}
	var buf bytes.Buffer
	buf.Write(cmdInit)
	// An empty body with OpenDrawer is a "drawer-only" job (cobrar sin imprimir):
	// emit just the drawer kick, no line feed, so no blank paper is ejected.
	if ta.Body != "" {
		buf.WriteString(ta.Body)
		buf.WriteByte('\n')
	}
	if opts.OpenDrawer {
		buf.Write(cmdDrawer)
	}
	if opts.Cut {
		buf.Write(escJFeed(opts.CutFeedDots)) // clear the cutter, then cut below content
		buf.Write(cmdFullCut)
	}
	return buf.Bytes(), nil
}
