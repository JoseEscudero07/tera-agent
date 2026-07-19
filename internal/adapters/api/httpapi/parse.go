package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

var errUnsupportedContentType = errors.New("unsupported content type (use multipart/form-data or application/json)")

// parseRequest reads a print request from JSON or multipart/form-data.
func parseRequest(r *http.Request) (printRequest, error) {
	ct := r.Header.Get("Content-Type")
	switch {
	case strings.HasPrefix(ct, "multipart/form-data"):
		return parseMultipart(r)
	case ct == "" || strings.HasPrefix(ct, "application/json"):
		var req printRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return printRequest{}, fmt.Errorf("invalid json: %w", err)
		}
		return req, nil
	default:
		return printRequest{}, errUnsupportedContentType
	}
}

// parseMultipart reads the `file` upload plus form fields.
func parseMultipart(r *http.Request) (printRequest, error) {
	if err := r.ParseMultipartForm(maxBody); err != nil {
		return printRequest{}, err
	}
	var req printRequest
	req.Printer = r.FormValue("printer")
	req.Format = r.FormValue("format")
	req.Paper = atoiDefault(r.FormValue("paper"), 0)
	req.Width = atoiDefault(r.FormValue("width"), 0)
	req.Drawer = r.FormValue("drawer") == "true"
	if v := r.FormValue("cut"); v != "" {
		b := v == "true"
		req.Cut = &b
	}

	file, hdr, err := r.FormFile("file")
	if err != nil {
		return printRequest{}, fmt.Errorf("missing 'file' field: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return printRequest{}, err
	}
	req.Content = data
	if req.Format == "" && hdr != nil {
		req.Format = formatFromName(hdr.Filename)
	}
	return req, nil
}

func atoiDefault(s string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	return def
}

func formatFromName(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pdf":
		return string(dp.FormatPDF)
	case ".png":
		return string(dp.FormatPNG)
	case ".jpg", ".jpeg":
		return string(dp.FormatJPEG)
	case ".txt":
		return string(dp.FormatText)
	default:
		return ""
	}
}
