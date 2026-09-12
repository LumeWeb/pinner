.PHONY: assets genappmanifest cssbuild

# assets regenerates all embeddable assets the canvasassets surface depends on:
# the mcpcanvas AssetSource manifest (genappmanifest) and the compiled Tailwind
# stylesheet (cssbuild). The per-app dist bundles themselves are produced by the
# shared apps build (pinner-cli's packages/apps, `pnpm build`) and staged into
# canvasassets/appsassets/dist/ — see jsbuild in pinner-cli. Those bundles are
# committed alongside the manifest/theme so a consuming module is
# self-contained; this target keeps the generated parts current.
assets: genappmanifest cssbuild

# genappmanifest regenerates canvasassets/appsassets/manifest.json against the
# bundles already in canvasassets/appsassets/dist/. Self-contained (no JS
# toolchain); used standalone when iterating on bundles without a full JS build
# and wired into CI so the committed manifest always reflects the dist.
genappmanifest:
	GOFLAGS=-mod=mod go run ./canvasassets/cmd/genappmanifest

# cssbuild compiles the MCP Apps Tailwind theme (canvasassets/css/input.css)
# into the embedded stylesheet (canvasassets/css/tailwind.css) that every ui://
# app inlines. Requires pnpm on PATH (see package.json build:css). Must run
# before any go build so the go:embed picks up the freshly compiled CSS.
cssbuild:
	pnpm build:css
