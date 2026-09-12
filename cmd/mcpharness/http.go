package main

// http.go mounts the HTTP-mode surface: the streamable-HTTP MCP transport
// behind a static bearer middleware, the mcpplane presigned transfer routes
// (their CORS is internal to mcpplane/transfer — the harness adds none of its
// own), and a trivial health check.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"go.lumeweb.com/mcpplane/sdk"
)

// HTTPServeConfig describes the HTTP serving surface.
type HTTPServeConfig struct {
	// Host is the bind host (loopback by default).
	Host string
	// Port is the bind port.
	Port int
	// AuthToken is the static bearer for /mcp. When empty /mcp serves
	// unauthenticated (anonymous local harness).
	AuthToken string
	// OAuth enables the test-only RFC 9728 authorization server in
	// cmd/mcpharness/oauth.go and protects /mcp with issued access tokens
	// instead of the static bearer.
	OAuth bool
}

// ServeHTTP serves the harness over streamable HTTP until ctx is cancelled.
func ServeHTTP(ctx context.Context, h *Harness, cfg HTTPServeConfig) error {
	base := "http://" + net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))

	// Stamp the base URL so presigned URLs minted by the app helpers point at
	// this listener rather than a stdio loopback.
	h.Upload.SetBaseURL(base)
	h.Download.SetBaseURL(base)

	mux := http.NewServeMux()

	// Presigned transfer routes: PUT /upload/<token> (202 + CORS reflect),
	// GET /download/<token>. CORS is handled inside mcpplane/transfer.
	h.Upload.RegisterHandlers(mux)
	h.Download.RegisterHandlers(mux)

	// MCP streamable-HTTP transport. In --oauth mode it is protected by the
	// test-only authorization server; otherwise by the optional static bearer.
	mcpHandler := sdk.NewStreamableHandler(h.SDK, false)
	switch {
	case cfg.OAuth:
		oauth := newOAuthProvider(base, cfg.AuthToken)
		oauth.registerHandlers(mux)
		mcpHandler = oauth.authorize(mcpHandler)
	case cfg.AuthToken != "":
		mcpHandler = bearerMiddleware(mcpHandler, cfg.AuthToken)
	}
	mux.Handle("/mcp", mcpHandler)

	// Liveness.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	// Startup banner goes to stderr; stdout must never carry harness logs.
	fmt.Fprintf(os.Stderr, "mcpharness: http listening on %s/mcp (transfers under %s/upload, /download)\n", base, base)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// bearerMiddleware enforces `Authorization: Bearer <token>` (static bearer).
// Wrong or missing credentials get a JSON 401 — it never adds CORS headers;
// those belong to the transfer routes only.
func bearerMiddleware(next http.Handler, token string) http.Handler {
	expected := "Bearer " + token
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != expected {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized: missing or invalid bearer token"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
