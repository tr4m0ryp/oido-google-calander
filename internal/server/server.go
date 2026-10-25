// Package server assembles the HTTP service: /mcp (Streamable HTTP behind a
// static bearer token), /health, and the admin pages.
package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tr4m0ryp/oido-google-calander/internal/server/admin"
	"github.com/tr4m0ryp/oido-google-calander/internal/server/admin/access"
)

// maxMCPBody caps one JSON-RPC request body.
const maxMCPBody = 1 << 20

// Options configure the HTTP handler.
type Options struct {
	MCP          *mcp.Server
	APIKey       string // empty disables bearer auth (explicit opt-in only)
	Admin        *admin.Admin
	AccountCount func() int
	Version      string
}

// New returns the root handler.
func New(o Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok", "accounts": o.AccountCount(), "version": o.Version,
		})
	})
	stream := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return o.MCP },
		&mcp.StreamableHTTPOptions{
			Stateless:    true,
			JSONResponse: true,
			// Behind Caddy the Host header is the public name while the peer is
			// local; the bearer token already defeats DNS-rebinding attacks.
			DisableLocalhostProtection: true,
		})
	mux.Handle("/mcp", bearer(o.APIKey, http.MaxBytesHandler(stream, maxMCPBody)))
	o.Admin.Register(mux)
	return logRequests(mux)
}

// bearer rejects /mcp requests without the static API key.
func bearer(key string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key != "" {
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || !access.SafeEqual(strings.TrimSpace(token), key) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="mcp"`)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32001,` +
					`"message":"Unauthorized: valid bearer token required"},"id":null}`))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// logRequests logs method, path (never the query) and status.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		if r.URL.Path == "/health" {
			return
		}
		slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.status,
			"ms", time.Since(start).Milliseconds())
	})
}
