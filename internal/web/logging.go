package web

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// SetupLogger configures the default slog logger to emit JSON lines on stderr.
// Level is read from PTM_LOG_LEVEL (debug|info|warn|error); default info.
// Output goes to stderr so systemd/journald captures it; JSON makes it
// scrape-friendly for Loki/Promtail/Vector → Grafana later.
func SetupLogger() *slog.Logger {
	lvl := slog.LevelInfo
	switch strings.ToLower(os.Getenv("PTM_LOG_LEVEL")) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	logger := slog.New(h).With("svc", "ptm-web")
	slog.SetDefault(logger)
	return logger
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// Flush forwards to the underlying writer if it supports flushing — required
// for SSE streaming on the chat API to work through this middleware.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// loggingMiddleware emits one structured log line per request.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"bytes", rec.bytes,
			"remote", r.RemoteAddr,
		)
	})
}
