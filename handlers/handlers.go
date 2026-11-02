package handlers

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/Shagrat2/go-autoconfig/config"
)

const maxBodyBytes = 1 << 20

var version = "devel"

type Handler struct {
	Config *config.Config
}

func NewRouter(c *config.Config) http.Handler {
	h := &Handler{Config: c}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.Index)
	mux.HandleFunc("POST /autodiscover/autodiscover.xml", h.Outlook)
	mux.HandleFunc("GET /autodiscover/autodiscover.xml", h.Outlook)
	mux.HandleFunc("GET /mail/config-v1.1.xml", h.Thunderbird)
	mux.HandleFunc("GET /.well-known/autoconfig/mail/config-v1.1.xml", h.Thunderbird)
	mux.HandleFunc("GET /healthz", h.Health)
	mux.HandleFunc("GET /version", h.Version)
	mux.HandleFunc("POST /autodiscover/autodiscover.svc", h.SOAP)
	if c.EnableApple {
		mux.HandleFunc("GET /email.mobileconfig", h.AppleMail)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p, ok := canonicalPath(r.URL.Path); ok {
			r.URL.Path = p
		}
		mux.ServeHTTP(w, r)
	})
}

var routes = []string{
	"/autodiscover/autodiscover.xml",
	"/autodiscover/autodiscover.svc",
	"/mail/config-v1.1.xml",
	"/.well-known/autoconfig/mail/config-v1.1.xml",
	"/email.mobileconfig",
	"/healthz",
	"/version",
}

func canonicalPath(p string) (string, bool) {
	for _, route := range routes {
		if strings.EqualFold(p, route) {
			return route, true
		}
	}

	return "", false
}

func (h *Handler) Outlook(w http.ResponseWriter, r *http.Request) {
	domain, d, ok := h.domain(r)
	if !ok {
		http.NotFound(w, r)
		return
	}

	var req struct {
		XMLName xml.Name `xml:"Autodiscover"`
		Request struct {
			EMailAddress             string `xml:"EMailAddress"`
			AcceptableResponseSchema string `xml:"AcceptableResponseSchema"`
		} `xml:"Request"`
	}

	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}

		http.Error(w, "cannot read request body", http.StatusBadRequest)
		return
	}

	if err := xml.Unmarshal(b, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	schema, ok := responseSchema(req.Request.AcceptableResponseSchema)
	if !ok {
		http.Error(w, "unsupported AcceptableResponseSchema", http.StatusBadRequest)
		return
	}

	out, err := marshalXML(buildAutodiscover(d, domain, req.Request.EMailAddress, schema))
	if err != nil {
		logError(r, "marshal autodiscover response", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	writeXML(w, out)
}

func (h *Handler) Thunderbird(w http.ResponseWriter, r *http.Request) {
	domain, d, ok := h.domain(r)
	if !ok {
		http.NotFound(w, r)
		return
	}

	out, err := marshalXML(buildClientConfig(d, domain))
	if err != nil {
		logError(r, "marshal autoconfig response", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	writeXML(w, out)
}

func (h *Handler) AppleMail(w http.ResponseWriter, r *http.Request) {
	domain, d, ok := h.domain(r)
	if !ok {
		http.NotFound(w, r)
		return
	}

	out, err := marshalPlist(buildAppleProfile(d, domain))
	if err != nil {
		logError(r, "marshal apple profile", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	w.Header().Set("Content-Disposition", `attachment; filename="email.mobileconfig"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
}

func (h *Handler) SOAP(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "SOAP Autodiscover is not supported; use POX", http.StatusBadRequest)
}

type versionInfo struct {
	Version  string `json:"version"`
	Commit   string `json:"commit,omitempty"`
	Time     string `json:"time,omitempty"`
	Modified string `json:"modified,omitempty"`
	Go       string `json:"go"`
}

func (h *Handler) Version(w http.ResponseWriter, r *http.Request) {
	info := versionInfo{Version: version, Go: runtime.Version()}
	if build, ok := debug.ReadBuildInfo(); ok {
		if build.Main.Version != "" && build.Main.Version != "(devel)" {
			info.Version = build.Main.Version
		}
		for _, setting := range build.Settings {
			switch setting.Key {
			case "vcs.revision":
				info.Commit = setting.Value
			case "vcs.time":
				info.Time = setting.Value
			case "vcs.modified":
				info.Modified = setting.Value
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(info)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok\n")
}

func (h *Handler) domain(r *http.Request) (string, config.Domain, bool) {
	host := hostname(r.Host)
	if host == "" {
		return "", config.Domain{}, false
	}

	for _, prefix := range []string{"autodiscover.", "autoconfig."} {
		if name, ok := strings.CutPrefix(host, prefix); ok {
			if d, found := h.Config.Domains[name]; found {
				return name, d, true
			}
		}
	}

	d, ok := h.Config.Domains[host]

	return host, d, ok
}

func hostname(hostport string) string {
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		hostport = host
	}
	hostport = strings.Trim(strings.TrimSuffix(hostport, "."), "[]")

	return strings.ToLower(hostport)
}

func logError(r *http.Request, msg string, err error) {
	slog.ErrorContext(r.Context(), msg,
		"request_id", RequestID(r.Context()),
		"host", r.Host,
		"path", r.URL.Path,
		"error", err,
	)
}

func writeXML(w http.ResponseWriter, out []byte) {
	w.Header().Set("Content-Type", "application/xml; charset=UTF-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
}
