package print

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/teraerp/tera-agent/internal/domain/job"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// JobHandler adapts a job.Job of kind PRINT into a PrintJob for the Engine. It
// satisfies app/dispatcher.Handler structurally (Handle method).
type JobHandler struct {
	engine *Engine
	roles  *RoleResolver
}

// NewJobHandler binds the print engine and the role resolver to a dispatcher
// handler. The resolver is optional: if nil, jobs without an explicit
// PrinterID will fail with a clear error (path automático deshabilitado).
func NewJobHandler(engine *Engine, roles *RoleResolver) *JobHandler {
	return &JobHandler{engine: engine, roles: roles}
}

// jobPayload is the wire contract of a PRINT job. Content is base64 in JSON.
type jobPayload struct {
	PrinterID string `json:"printer_id"`
	// Role viaja cuando el backend hace impresión automática por rol y no
	// conoce el nombre físico de la impresora. Si viene con PrinterID vacío,
	// el Agent lo resuelve localmente con RoleResolver.
	Role       string `json:"role"`
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
	printerID := p.PrinterID
	if printerID == "" {
		if h.roles == nil {
			return fmt.Errorf("job sin printer_id y sin resolver de roles configurado")
		}
		printerID = h.roles.Resolve(p.Role)
		if printerID == "" {
			return fmt.Errorf(
				"no se pudo resolver la impresora para role=%q "+
					"(ni predeterminada del agente configurada)", p.Role,
			)
		}
	}
	return h.engine.Print(ctx, dp.PrintJob{
		PrinterID: printerID,
		Format:    dp.SourceFormat(p.Format),
		Content:   p.Content,
		Options: dp.Options{
			Copies:     p.Copies,
			Cut:        p.Cut,
			OpenDrawer: p.OpenDrawer,
		},
	})
}
