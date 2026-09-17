// Package manifest implementa update.Source consultando el endpoint de versión
// del Backend por HTTPS. Deriva la URL base del mismo dominio que el WebSocket:
// wss://host[/ruta] → https://host/agent/version. Owner: DevOps Engineer.
package manifest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/teraerp/tera-agent/internal/app/ports"
	du "github.com/teraerp/tera-agent/internal/domain/update"
)

// endpointPath es la ruta del manifiesto bajo el dominio del Backend.
const endpointPath = "/agent/version"

// Client consulta el manifiesto de versión. Reutiliza un http.Client con timeout
// acotado: la consulta no debe colgar el arranque ni el panel.
type Client struct {
	versionURL string
	http       *http.Client
	log        ports.Logger
}

// New construye el cliente derivando la URL del manifiesto a partir de la URL
// del Backend (la misma wss:// del agente). backendURL vacío → error explícito
// (modo local sin Backend: no hay de dónde actualizar).
func New(backendURL string, log ports.Logger) (*Client, error) {
	base, err := HTTPBase(backendURL)
	if err != nil {
		return nil, err
	}
	return &Client{
		versionURL: base + endpointPath,
		http:       &http.Client{Timeout: 15 * time.Second},
		log:        log,
	}, nil
}

// Fetch consulta {base}/agent/version?os=&arch=&current= y decodifica el JSON.
func (c *Client) Fetch(ctx context.Context, current, os, arch string) (du.Manifest, error) {
	u, err := url.Parse(c.versionURL)
	if err != nil {
		return du.Manifest{}, err
	}
	q := u.Query()
	q.Set("os", os)
	q.Set("arch", arch)
	q.Set("current", current)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return du.Manifest{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return du.Manifest{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return du.Manifest{}, fmt.Errorf("manifest: HTTP %d desde %s", resp.StatusCode, endpointPath)
	}
	var m du.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return du.Manifest{}, fmt.Errorf("manifest: JSON inválido: %w", err)
	}
	if m.Latest == "" {
		return du.Manifest{}, fmt.Errorf("manifest: respuesta sin campo 'latest'")
	}
	return m, nil
}

var _ du.Source = (*Client)(nil)

// HTTPBase deriva el origen HTTP(S) a partir de la URL del Backend. Convierte el
// esquema WebSocket al HTTP equivalente y descarta la ruta (nos quedamos con
// esquema://host[:puerto]):
//
//	wss://api.grupotera.cloud/ws/agent/  → https://api.grupotera.cloud
//	ws://localhost:8000/ws/agent/        → http://localhost:8000
//	https://api.grupotera.cloud          → https://api.grupotera.cloud
func HTTPBase(backendURL string) (string, error) {
	if backendURL == "" {
		return "", fmt.Errorf("manifest: BackendURL vacío (modo local: no hay servidor de actualización)")
	}
	u, err := url.Parse(backendURL)
	if err != nil {
		return "", fmt.Errorf("manifest: BackendURL inválida: %w", err)
	}
	scheme := u.Scheme
	switch scheme {
	case "wss", "https":
		scheme = "https"
	case "ws", "http":
		scheme = "http"
	default:
		return "", fmt.Errorf("manifest: esquema no soportado %q", u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("manifest: BackendURL sin host")
	}
	return scheme + "://" + u.Host, nil
}
