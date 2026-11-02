package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Shagrat2/go-autoconfig/config"
	"github.com/Shagrat2/go-autoconfig/handlers"
)

func TestPprofServerServesProfiles(t *testing.T) {
	srv := newPprofServer("127.0.0.1:0")

	for _, path := range []string{
		"/debug/pprof/",
		"/debug/pprof/cmdline",
		"/debug/pprof/goroutine?debug=1",
		"/debug/pprof/heap?debug=1",
	} {
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want %d", path, rec.Code, http.StatusOK)
		}
	}
}

func TestPublicRouterDoesNotExposePprof(t *testing.T) {
	cfg := &config.Config{
		Domains: map[string]config.Domain{
			"example.com": {
				IMAP: config.Server{Host: "imap.example.com", Port: 993},
				SMTP: config.Server{Host: "smtp.example.com", Port: 465},
			},
		},
	}

	r := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	r.Host = "example.com"
	rec := httptest.NewRecorder()
	handlers.NewRouter(cfg).ServeHTTP(rec, r)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRequestIDValidation(t *testing.T) {
	for _, id := range []string{"abc", "abc-123.X_y", strings.Repeat("a", 64)} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Request-ID", id)

		if got := requestID(r); got != id {
			t.Errorf("requestID(%q) = %q, want %q", id, got, id)
		}
	}

	for _, id := range []string{"bad id", "bad\nid", strings.Repeat("a", 65)} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Request-ID", id)

		got := requestID(r)
		if got == "" || got == id || !requestIDPattern.MatchString(got) {
			t.Errorf("requestID(%q) = %q, want generated id", id, got)
		}
	}
}

func TestLoggingPropagatesRequestID(t *testing.T) {
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })

	var got string
	h := logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = handlers.RequestID(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	r := httptest.NewRequest(http.MethodGet, "/mail/config-v1.1.xml", nil)
	r.Header.Set("X-Request-ID", "abc-123")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	if got != "abc-123" {
		t.Errorf("context request id = %q, want %q", got, "abc-123")
	}
	if header := rec.Header().Get("X-Request-ID"); header != "abc-123" {
		t.Errorf("response X-Request-ID = %q, want %q", header, "abc-123")
	}
}

func TestRateLimiterAllow(t *testing.T) {
	limiter := newRateLimiter(2, false)
	now := time.Unix(0, 0)

	if !limiter.allow("a", now) {
		t.Fatal("first request must be allowed")
	}
	if !limiter.allow("a", now) {
		t.Fatal("second request must be allowed")
	}
	if limiter.allow("a", now) {
		t.Fatal("third request must be limited")
	}
	if !limiter.allow("b", now) {
		t.Fatal("different client must be allowed")
	}

	now = now.Add(time.Minute)
	if !limiter.allow("a", now) {
		t.Fatal("window must reset after a minute")
	}
}

func TestRateLimiterDisabled(t *testing.T) {
	if newRateLimiter(0, false) != nil {
		t.Fatal("rate limiter must be disabled for limit 0")
	}
	if newRateLimiter(-1, false) != nil {
		t.Fatal("rate limiter must be disabled for negative limit")
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.1:1234"

	if got := clientIP(r, false); got != "192.0.2.1" {
		t.Errorf("clientIP = %q, want %q", got, "192.0.2.1")
	}

	r.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
	if got := clientIP(r, false); got != "192.0.2.1" {
		t.Errorf("untrusted X-Forwarded-For used: %q", got)
	}
	if got := clientIP(r, true); got != "203.0.113.5" {
		t.Errorf("trusted X-Forwarded-For = %q, want %q", got, "203.0.113.5")
	}

	r.Header.Set("X-Forwarded-For", "not-an-ip")
	if got := clientIP(r, true); got != "192.0.2.1" {
		t.Errorf("invalid X-Forwarded-For fallback = %q", got)
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	limiter := newRateLimiter(1, false)
	h := limiter.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "192.0.2.1:1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}

	if code := request().Code; code != http.StatusNoContent {
		t.Fatalf("first status = %d", code)
	}
	rec := request()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if got := rec.Header().Get("Retry-After"); got != "60" {
		t.Errorf("Retry-After = %q, want 60", got)
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	for _, header := range []string{"X-Content-Type-Options", "Referrer-Policy", "X-Frame-Options"} {
		if rec.Header().Get(header) == "" {
			t.Errorf("%s header is missing", header)
		}
	}
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS must not be set for plain HTTP")
	}

	rec = httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	h.ServeHTTP(rec, r)
	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Error("HSTS header is missing for HTTPS")
	}
}
