package printing

import (
	"context"
	"encoding/json"

	"github.com/teraerp/tera-agent/internal/app/ports"
	"github.com/teraerp/tera-agent/internal/domain/job"
	domainprinting "github.com/teraerp/tera-agent/internal/domain/printing"
)

// Handler executes PRINT jobs: it decodes the job payload into a print Request
// and forwards it to the printing Port. It satisfies app/dispatcher.Handler
// structurally (Handle method), without importing the app layer.
// Owner: Printing Engineer.
type Handler struct {
	port domainprinting.Port
	log  ports.Logger
}

// NewHandler binds the printing Port to a dispatcher handler.
func NewHandler(port domainprinting.Port, log ports.Logger) *Handler {
	return &Handler{port: port, log: log}
}

// Handle decodes the request and prints it.
func (h *Handler) Handle(ctx context.Context, j job.Job) error {
	var req domainprinting.Request
	if err := json.Unmarshal(j.Payload, &req); err != nil {
		return err
	}
	h.log.Debug("handling print job", "id", j.ID, "printer", req.PrinterID, "raw", req.Raw)
	return h.port.Print(ctx, domainprinting.Document{
		PrinterID: req.PrinterID,
		Data:      req.Data,
		Raw:       req.Raw,
	})
}
