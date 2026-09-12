// Command mcpharness is a runnable, test-only MCP server harness for pinner.
// It stands up the complete pinner MCP surface (progressive-disclosure
// meta-tools, compiled catalog tools, direct tools, and ui:// MCP Apps backed
// by real canvasassets documents) over FAKED core services — the harness never
// touches the network.
//
// Modes:
//
//	mcpharness                stdio transport (MCP over stdin/stdout)
//	mcpharness --http         streamable HTTP at http://<host>:<port>/mcp
//	                          (behind a static bearer when --auth-token is set)
//
// The presigned transfer coordinators (PUT /upload/<token>, GET
// /download/<token>) are mounted on the HTTP mux in HTTP mode and served on an
// automatic loopback listener in stdio mode (mcpplane's EnsureLoopback), so
// the app upload helpers mint reachable URLs in both modes.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	log "go.uber.org/zap"

	"go.lumeweb.com/mcpplane/sdk"
)

func main() {
	var (
		port      = flag.Int("port", 8126, "HTTP mode: port to listen on")
		httpMode  = flag.Bool("http", false, "serve the streamable-HTTP transport instead of stdio")
		oauthMode = flag.Bool("oauth", false, "HTTP mode: protect /mcp with the test-only OAuth authorization server instead of the static bearer")
		authToken = flag.String("auth-token", "", "static bearer / OAuth resource-owner secret for the HTTP /mcp route and the faked credential (default: token-<seed-email>)")
		host      = flag.String("host", "127.0.0.1", "HTTP mode: bind host")
		seedEmail = flag.String("seed-email", "agent@example.com", "seed account email for the in-memory faked services")
	)
	flag.Parse()

	// Stdio mode is the process's only log surface: everything goes to stderr,
	// never stdout (which carries MCP frames).
	logger, _ := log.NewDevelopment()
	defer func() { _ = logger.Sync() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	opts := HarnessOptions{
		AuthToken: *authToken,
		SeedEmail: *seedEmail,
	}
	harness, err := Build(ctx, opts)
	if err != nil {
		logger.Fatal("mcpharness: build failed", log.Error(err))
	}

	if *httpMode {
		cfg := HTTPServeConfig{
			Host:      *host,
			Port:      *port,
			AuthToken: harness.Fakes.Token,
			OAuth:     *oauthMode,
		}
		if err := ServeHTTP(ctx, harness, cfg); err != nil {
			logger.Fatal("mcpharness: http serve failed", log.Error(err))
		}
		return
	}

	// Stdio: the presigned upload coordinator spins its loopback listener
	// lazily on first Mint/Prepare, so no explicit mount is needed here.
	fmt.Fprintf(os.Stderr, "mcpharness: stdio MCP serving (base endpoint %s, account %s)\n", fakeBaseEndpoint, harness.Fakes.Email)
	if err := sdk.RunStdio(ctx, harness.SDK, os.Stdin, os.Stdout); err != nil {
		logger.Fatal("mcpharness: stdio serve failed", log.Error(err))
	}
}
