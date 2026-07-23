package print

import (
	"context"
	"strings"
	"testing"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// --- fakes implementing the domain ports ---

type fakeRenderer struct {
	src      dp.SourceFormat
	produces dp.ArtifactKind
}

func (f fakeRenderer) CanRender(s dp.SourceFormat) bool { return s == f.src }
func (f fakeRenderer) Produces() dp.ArtifactKind        { return f.produces }
func (f fakeRenderer) Render(context.Context, []byte, dp.RenderOptions) (dp.Artifact, error) {
	return dp.TextArtifact{}, nil
}

type fakeEncoder struct {
	accepts  dp.ArtifactKind
	produces dp.DeviceFormat
}

func (f fakeEncoder) Accepts() dp.ArtifactKind  { return f.accepts }
func (f fakeEncoder) Produces() dp.DeviceFormat { return f.produces }
func (f fakeEncoder) Encode(context.Context, dp.Artifact, dp.EncodeOptions) ([]byte, error) {
	return nil, nil
}

type fakeDriver struct{ accepts dp.DeviceFormat }

func (f fakeDriver) Accepts(d dp.DeviceFormat) bool             { return d == f.accepts }
func (f fakeDriver) Send(context.Context, string, []byte) error { return nil }

func TestResolver_ComposesRasterPipeline(t *testing.T) {
	ren := fakeRenderer{src: dp.FormatPDF, produces: dp.ArtifactRaster}
	enc := fakeEncoder{accepts: dp.ArtifactRaster, produces: dp.DeviceESCPOS}
	drv := fakeDriver{accepts: dp.DeviceESCPOS}

	r := NewResolver([]dp.Renderer{ren}, []dp.Encoder{enc}, []dp.Driver{drv})

	pipe, err := r.Resolve(dp.FormatPDF, dp.PrinterProfile{
		PrinterID:     "T",
		NativeFormats: []dp.DeviceFormat{dp.DeviceESCPOS},
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if pipe.Encoder.Produces() != dp.DeviceESCPOS {
		t.Fatalf("encoder produces %s, want escpos", pipe.Encoder.Produces())
	}
	if !pipe.Renderer.CanRender(dp.FormatPDF) {
		t.Fatal("renderer cannot render PDF")
	}
}

func TestResolver_PassThroughWhenSourceIsNative(t *testing.T) {
	drv := fakeDriver{accepts: dp.DevicePDF}
	r := NewResolver(nil, nil, []dp.Driver{drv})

	pipe, err := r.Resolve(dp.FormatPDF, dp.PrinterProfile{
		PrinterID:     "L",
		NativeFormats: []dp.DeviceFormat{dp.DevicePDF},
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if pipe.Encoder.Produces() != dp.DevicePDF {
		t.Fatalf("pass-through encoder produces %s, want pdf", pipe.Encoder.Produces())
	}
}

func TestResolver_NoPipeline(t *testing.T) {
	// A ZPL-only printer with no encoder producing ZPL cannot handle a PDF.
	drv := fakeDriver{accepts: dp.DeviceZPL}
	r := NewResolver(nil, nil, []dp.Driver{drv})

	_, err := r.Resolve(dp.FormatPDF, dp.PrinterProfile{
		PrinterID:     "Z",
		NativeFormats: []dp.DeviceFormat{dp.DeviceZPL},
	})
	if err == nil {
		t.Fatal("expected error resolving PDF for a ZPL-only printer, got nil")
	}
}

// Los tres tests siguientes verifican que Resolve devuelve un mensaje útil a
// la UI: identifica el formato del origen, la impresora y por qué no encaja.
// El texto exacto puede cambiar; lo que se verifica son las señales concretas
// que el usuario debe leer en el toast.

func TestResolver_ErrorMentionsPrinterAndFormat(t *testing.T) {
	drv := fakeDriver{accepts: dp.DeviceZPL}
	r := NewResolver(nil, nil, []dp.Driver{drv})

	_, err := r.Resolve(dp.FormatPDF, dp.PrinterProfile{
		PrinterID:     "Zebra-1",
		NativeFormats: []dp.DeviceFormat{dp.DeviceZPL},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Zebra-1") || !strings.Contains(msg, string(dp.FormatPDF)) {
		t.Fatalf("error should mention printer and source format, got: %q", msg)
	}
}

func TestResolver_ErrorFlagsMissingDriver(t *testing.T) {
	// El perfil pide GDIRaster pero la plataforma no tiene driver para él.
	r := NewResolver(nil, nil, nil)
	_, err := r.Resolve(dp.FormatPDF, dp.PrinterProfile{
		PrinterID:     "HP",
		NativeFormats: []dp.DeviceFormat{dp.DeviceGDIRaster},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "driver") {
		t.Fatalf("error should explain missing driver, got: %q", err.Error())
	}
}

func TestResolver_ErrorFlagsMissingEncoder(t *testing.T) {
	// Hay driver, pero ningún encoder produce ese target.
	drv := fakeDriver{accepts: dp.DeviceGDIRaster}
	r := NewResolver(nil, nil, []dp.Driver{drv})
	_, err := r.Resolve(dp.FormatPDF, dp.PrinterProfile{
		PrinterID:     "HP",
		NativeFormats: []dp.DeviceFormat{dp.DeviceGDIRaster},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "encoder") {
		t.Fatalf("error should explain missing encoder, got: %q", err.Error())
	}
}

func TestResolver_EmptyNativeFormats(t *testing.T) {
	_, err := NewResolver(nil, nil, nil).Resolve(dp.FormatPDF, dp.PrinterProfile{
		PrinterID: "X",
	})
	if err == nil {
		t.Fatal("expected error for profile without native formats")
	}
	if !strings.Contains(err.Error(), "X") {
		t.Fatalf("error should mention the printer, got: %q", err.Error())
	}
}
