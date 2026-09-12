package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGeneratedManifestMatchesCommitted pins that the committed manifest is in
// sync with the embedded dist bundles. CI regenerates the manifest to a temp
// file and diffs against the committed one (see go.yml); this test asserts the
// same invariant directly, so a stale committed manifest fails the Go test
// suite even outside the CI diff step. Out-of-sync manifests cause runtime
// handshake failures for consumers, so hardening this guard is a regression
// against the "regenerate in place masks drift" failure mode.
func TestGeneratedManifestMatchesCommitted(t *testing.T) {
	root, err := appsAssetsRoot()
	if err != nil {
		t.Fatalf("resolve assets root: %v", err)
	}
	committed, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatalf("read committed manifest: %v", err)
	}

	out := filepath.Join(t.TempDir(), "manifest.json")
	if err := run(out); err != nil {
		t.Fatalf("generate manifest to %s: %v", out, err)
	}
	generated, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read generated manifest: %v", err)
	}
	if string(committed) != string(generated) {
		t.Errorf("committed canvasassets/appsassets/manifest.json is out of sync with the dist bundles — run `go run ./canvasassets/cmd/genappmanifest`")
	}
}
