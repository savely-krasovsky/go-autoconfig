package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Shagrat2/go-autoconfig/config"
	"github.com/Shagrat2/go-autoconfig/handlers"
)

var path = flag.String(
	"config",
	"",
	"enter path to config file",
)

func main() {
	flag.Parse()

	if err := run(); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	conf, err := config.NewConfig(*path)
	if err != nil {
		return err
	}
	if listen := os.Getenv("AUTOCONFIG_LISTEN"); listen != "" {
		conf.Listen = listen
	}
	if pprofListen := os.Getenv("AUTOCONFIG_PPROF"); pprofListen != "" {
		if pprofListen == "1" {
			pprofListen = "127.0.0.1:6060"
		}
		conf.PprofListen = pprofListen
	}
	if err := conf.Validate(); err != nil {
		return err
	}

	limiter := newRateLimiter(conf.RateLimit, conf.TrustProxy)

	srv := &http.Server{
		Addr:              conf.Listen,
		Handler:           securityHeaders(logging(recoverPanics(limiter.middleware(handlers.NewRouter(conf))))),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	servers := []*http.Server{srv}
	if conf.PprofListen != "" {
		pprofSrv := newPprofServer(conf.PprofListen)
		servers = append(servers, pprofSrv)

		go func() {
			slog.Info("pprof started", "listen", conf.PprofListen)
			if err := pprofSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("pprof server failed", "error", err)
			}
		}()
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server started", "listen", conf.Listen)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		return err
	case <-ctx.Done():
		slog.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, s := range servers {
		if err := s.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown %s: %w", s.Addr, err)
		}
	}

	return nil
}

func newPprofServer(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}

	return r.ResponseWriter.Write(b)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}

type rateWindow struct {
	start time.Time
	count int
}

type rateLimiter struct {
	limit      int
	trustProxy bool
	requests   uint64

	mu      sync.Mutex
	clients map[string]*rateWindow
}

func newRateLimiter(limit int, trustProxy bool) *rateLimiter {
	if limit <= 0 {
		return nil
	}

	return &rateLimiter{
		limit:      limit,
		trustProxy: trustProxy,
		clients:    make(map[string]*rateWindow),
	}
}

func (l *rateLimiter) middleware(next http.Handler) http.Handler {
	if l == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r, l.trustProxy), time.Now()) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (l *rateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.requests++
	if l.requests%1024 == 0 {
		l.cleanup(now)
	}

	window, ok := l.clients[key]
	if !ok || now.Sub(window.start) >= time.Minute {
		l.clients[key] = &rateWindow{start: now, count: 1}
		return true
	}
	if window.count >= l.limit {
		return false
	}
	window.count++

	return true
}

func (l *rateLimiter) cleanup(now time.Time) {
	for key, window := range l.clients {
		if now.Sub(window.start) >= 2*time.Minute {
			delete(l.clients, key)
		}
	}
}

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			first, _, _ := strings.Cut(forwarded, ",")
			if ip := net.ParseIP(strings.TrimSpace(first)); ip != nil {
				return ip.String()
			}
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := requestID(r)
		r = r.WithContext(handlers.WithRequestID(r.Context(), id))
		w.Header().Set("X-Request-ID", id)

		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}

		slog.Info("request",
			"request_id", id,
			"method", r.Method,
			"host", r.Host,
			"path", r.URL.Path,
			"status", status,
			"duration", time.Since(start),
		)
	})
}

func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered", "error", rec, "path", r.URL.Path)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func requestID(r *http.Request) string {
	if id := r.Header.Get("X-Request-ID"); requestIDPattern.MatchString(id) {
		return id
	}

	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}

	return hex.EncodeToString(b[:])
}
