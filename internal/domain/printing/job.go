package printing

// PrintJob is what the print Engine receives. The ERP sends only the printer_id
// plus the document; the Agent already knows the printer's profile (cached).
type PrintJob struct {
	PrinterID string
	Format    SourceFormat
	Content   []byte
	Options   Options
}

// Options are per-job toggles that the profile must also support.
type Options struct {
	Copies     int
	Cut        bool
	OpenDrawer bool
}

// RenderOptions parameterize a Renderer (e.g. rasterize to the native width).
type RenderOptions struct {
	WidthDots int
	DPI       int
	Color     bool
}

// EncodeOptions parameterize an Encoder (cut, drawer, width).
type EncodeOptions struct {
	WidthDots  int
	Cut        bool
	OpenDrawer bool
	// CutFeedDots is how much paper to feed (ESC J) before the cut so the last
	// line clears the blade. Per-printer (blade distance varies by model). 0 lets
	// the encoder use its built-in default.
	CutFeedDots int
	// TopMarginDots is how many blank top rows to keep (the rest is trimmed) so
	// the receipt starts close to the content. 0 lets the encoder use its default.
	TopMarginDots int
}

// RasterOptions parameterize a Rasterizer.
type RasterOptions struct {
	WidthDots int
	DPI       int
	Gray      bool
}
