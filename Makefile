.PHONY: build test clean assets jsbuild cssbuild genappmanifest templinstall generate

# A bare `make` must produce a build, not just regenerate assets. Pin the
# default explicitly so reordering rules later can't regress it.
.DEFAULT_GOAL := build

# assets regenerates every embeddable asset the canvasassets surface depends
# on: the per-app JS bundles (jsbuild), the compiled Tailwind theme (cssbuild),
# and the mcpcanvas manifest (genappmanifest). It must run before any go
# build/test so the go:embed directives pick up freshly generated files.
assets: jsbuild cssbuild genappmanifest

# jsbuild builds the MCP App JS bundles from this module's own packages/apps
# (pnpm build via tsdown) and stages the built dist bundles into
# canvasassets/appsassets/dist/ so Go embeds them. pinner owns the full JS
# ecosystem (source, workspace, Tailwind build), so nothing external is needed.
# Requires pnpm on PATH. Go build/test embed these bundles, so jsbuild must run
# before any go build/test.
jsbuild:
	pnpm install --frozen-lockfile
	pnpm build
	mkdir -p canvasassets/appsassets/dist
	cp packages/apps/dist/*.js canvasassets/appsassets/dist/

# cssbuild compiles the MCP Apps Tailwind theme (canvasassets/css/input.css)
# into the embedded stylesheet (canvasassets/css/tailwind.css) that every ui://
# app inlines. Requires pnpm on PATH (see package.json build:css). Must run
# before any go build so the go:embed picks up the freshly compiled CSS.
cssbuild:
	pnpm build:css

# genappmanifest regenerates canvasassets/appsassets/manifest.json against the
# bundles already in canvasassets/appsassets/dist/. Self-contained (no JS
# toolchain). Run after jsbuild as part of `assets`.
genappmanifest:
	GOFLAGS=-mod=mod go run ./canvasassets/cmd/genappmanifest

# templinstall installs the templ CLI used by `generate`, pinned to the version
# declared in go.mod (github.com/a-h/templ).
templinstall:
	go install github.com/a-h/templ/cmd/templ@v0.3.1020

# generate regenerates the templ-derived *_templ.go files (the canvas view
# bodies). Runs before build so a change to a *.templ file is never built with
# stale generated output.
generate:
	templ generate

build: assets
	go build -v ./...

test: assets
	go test -v ./...

# mcpharness builds the runnable MCP server harness (cmd/mcpharness) that the
# sunpeak integration suite (tests/sunpeak) drives over stdio. It stands up the
# complete pinner MCP surface over faked core services — no network — so it is
# the single, in-module test substrate for the MCP Apps render/asset seam.
.PHONY: mcpharness
mcpharness:
	mkdir -p bin
	go build -o bin/mcpharness ./cmd/mcpharness
