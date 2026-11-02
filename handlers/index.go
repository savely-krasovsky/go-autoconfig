package handlers

import (
	"bytes"
	_ "embed"
	"html/template"
	"net/http"

	"github.com/Shagrat2/go-autoconfig/config"
)

//go:embed index.html
var indexHTML string

var indexTemplate = template.Must(template.New("index").Parse(indexHTML))

type indexServer struct {
	Host     string
	Port     int
	Security string
}

type indexData struct {
	Project      string
	Version      string
	Domain       string
	DisplayName  string
	IMAP         indexServer
	SMTP         indexServer
	AppleEnabled bool
}

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	domain, d, ok := h.domain(r)
	if !ok {
		http.NotFound(w, r)
		return
	}

	displayName := d.DisplayName
	if displayName == "" {
		displayName = domain
	}

	data := indexData{
		Project:      "go-autoconfig",
		Version:      version,
		Domain:       domain,
		DisplayName:  displayName,
		IMAP:         newIndexServer(d.IMAP),
		SMTP:         newIndexServer(d.SMTP),
		AppleEnabled: h.Config.EnableApple,
	}

	var body bytes.Buffer
	if err := indexTemplate.Execute(&body, data); err != nil {
		logError(r, "render index page", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body.Bytes())
}

func newIndexServer(s config.Server) indexServer {
	security := s.SocketType
	if security == "" {
		security = "SSL"
		if s.STARTTLS {
			security = "STARTTLS"
		}
	}

	return indexServer{
		Host:     s.Host,
		Port:     s.Port,
		Security: security,
	}
}
