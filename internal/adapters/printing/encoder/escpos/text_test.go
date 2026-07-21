package escpos

import (
	"bytes"
	"context"
	"testing"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

func TestTextEncoder_InitBodyAndFeedBeforeCut(t *testing.T) {
	enc := NewText()
	out, err := enc.Encode(context.Background(), dp.TextArtifact{Body: "HOLA"}, dp.EncodeOptions{Cut: true})
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}
	if !bytes.HasPrefix(out, []byte{0x1B, 0x40}) { // ESC @ init
		t.Error("must start with ESC @ init")
	}
	if !bytes.Contains(out, []byte("HOLA")) {
		t.Error("missing body")
	}
	// Feed (ESC J) must appear before the final cut so the last line clears the
	// blade instead of being left dangling on the next ticket.
	feed := bytes.Index(out, []byte{0x1B, 0x4A})
	cut := bytes.Index(out, []byte{0x1D, 0x56, 0x00})
	if feed < 0 {
		t.Fatal("missing ESC J feed before cut")
	}
	if cut < 0 || feed >= cut {
		t.Fatalf("feed (%d) must come before cut (%d)", feed, cut)
	}
	if !bytes.HasSuffix(out, []byte{0x1D, 0x56, 0x00}) {
		t.Error("must end with GS V 0 cut")
	}
}

func TestTextEncoder_NoCutNoFeed(t *testing.T) {
	enc := NewText()
	out, err := enc.Encode(context.Background(), dp.TextArtifact{Body: "X"}, dp.EncodeOptions{})
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}
	if bytes.Contains(out, []byte{0x1D, 0x56, 0x00}) {
		t.Error("must not cut when Cut is false")
	}
	if bytes.Contains(out, []byte{0x1B, 0x4A}) {
		t.Error("must not feed-before-cut when Cut is false")
	}
}

func TestTextEncoder_DrawerOnly_EmptyBodyNoFeed(t *testing.T) {
	// Cuerpo vacío + OpenDrawer = "abrir cajón sin imprimir": solo init + ESC p,
	// sin salto de línea (no debe expulsar papel) y sin corte.
	enc := NewText()
	out, err := enc.Encode(
		context.Background(),
		dp.TextArtifact{Body: ""},
		dp.EncodeOptions{OpenDrawer: true},
	)
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}
	// ESC @ (init) + ESC p (drawer), nada más.
	want := []byte{0x1B, 0x40, 0x1B, 0x70, 0x00, 0x19, 0xFA}
	if !bytes.Equal(out, want) {
		t.Fatalf("drawer-only bytes = %v, want %v", out, want)
	}
	if bytes.Contains(out, []byte{'\n'}) {
		t.Error("no debe alimentar papel (\\n) en un trabajo solo-cajón")
	}
}

func TestTextEncoder_RejectsWrongArtifact(t *testing.T) {
	enc := NewText()
	if _, err := enc.Encode(context.Background(), dp.RasterArtifact{}, dp.EncodeOptions{}); err == nil {
		t.Fatal("expected error for non-text artifact, got nil")
	}
}
