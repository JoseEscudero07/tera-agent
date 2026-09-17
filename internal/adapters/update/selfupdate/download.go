// Package selfupdate implementa update.Applier: descarga el binario nuevo, lo
// verifica por SHA256 y reemplaza el ejecutable en marcha, reiniciando el
// Agent. La verificación del hash es OBLIGATORIA antes de tocar el binario.
// Owner: DevOps Engineer + Security Engineer.
package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// downloadVerified descarga url a destPath calculando el SHA256 al vuelo y
// falla (borrando el fichero) si no coincide con wantHex. Escribir en el mismo
// directorio que el ejecutable permite luego un os.Rename atómico (mismo
// volumen), evitando fallos de "cross-device link".
func downloadVerified(ctx context.Context, client *http.Client, url, wantHex, destPath string) error {
	if wantHex == "" {
		return fmt.Errorf("selfupdate: SHA256 vacío; no se descarga sin verificación")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("selfupdate: descarga HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, h), resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		os.Remove(destPath)
		return copyErr
	}
	if closeErr != nil {
		os.Remove(destPath)
		return closeErr
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, strings.TrimSpace(wantHex)) {
		os.Remove(destPath)
		return fmt.Errorf("selfupdate: SHA256 no coincide (esperado %s, obtenido %s)", wantHex, got)
	}
	return nil
}
