package httputil

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"doc-agents/internal/app"
)

// NewRouter creates a chi router with standard middleware (RequestID, Recoverer, Logger, Timeout, RealIP).
func NewRouter(log *slog.Logger) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(Recoverer(log))
	r.Use(RequestLogger(log))

	return r
}

// WriteJSON writes a JSON response with proper headers.
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(body)
}

// HealthHandler returns a simple health check endpoint.
func HealthHandler(deps app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			deps.Log.Warn("healthz write failed", "err", err)
		}
	}
}

// RequestLogger is a lightweight HTTP logger that uses slog.
func RequestLogger(log *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}

// Recoverer logs panics via slog while preserving chi's Recoverer behavior.
func Recoverer(log *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic recovered", "panic", rec, "path", r.URL.Path, "method", r.Method, "request_id", middleware.GetReqID(r.Context()))
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Fail writes an error response with consistent logging.
func Fail(log *slog.Logger, w http.ResponseWriter, message string, err error, status int) {
	log.Error(message, "err", err)
	if status == 0 {
		status = http.StatusInternalServerError
	}
	http.Error(w, message, status)
}
