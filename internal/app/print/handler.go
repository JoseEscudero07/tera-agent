package print

import (
	"context"
	"encoding/json"

	"github.com/teraerp/tera-agent/internal/domain/job"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// JobHandler adapts a job.Job of kind PRINT into a PrintJob for the Engine. It
// satisfies app/dispatcher.Handler structurally (Handle method).
type JobHandler struct{ engine *Engine }

// NewJobHandler binds the print engine to a dispatcher handler.
func NewJobHandler(engine *Engine) *JobHandler { return &JobHandler{engine: engine} }

// jobPayload is the wire contract of a PRINT job. Content is base64 in JSON.
type jobPayload struct {
	PrinterID  string `json:"printer_id"`
	Format     string `json:"format"` // MIME source format, e.g. "application/pdf"
	Content    []byte `json:"content"`
	Copies     int    `json:"copies"`
	Cut        bool   `json:"cut"`
	OpenDrawer bool   `json:"open_drawer"`
}

// Handle decodes the payload and runs the print engine.
func (h *JobHandler) Handle(ctx context.Context, j job.Job) error {
	var p jobPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return err
	}
	return h.engine.Print(ctx, dp.PrintJob{
		PrinterID: p.PrinterID,
		Format:    dp.SourceFormat(p.Format),
		Content:   p.Content,
		Options: dp.Options{
			Copies:     p.Copies,
			Cut:        p.Cut,
			OpenDrawer: p.OpenDrawer,
		},
	})
}
