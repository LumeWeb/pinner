package hosted

import (
	"context"
	"testing"

	"go.lumeweb.com/pinner/assembly"
)

func bundleFactory(r CredentialResolver) func() *assembly.CatalogDepsBundle {
	return func() *assembly.CatalogDepsBundle {
		return &assembly.CatalogDepsBundle{CredentialResolver: r}
	}
}

func TestNormalize(t *testing.T) {
	t.Run("reconciles explicit resolver with bundle resolver", func(t *testing.T) {
		explicit := staticResolver{token: "a"}
		n, err := Normalize(Config{
			CatalogDeps:        bundleFactory(nil),
			CredentialResolver: explicit,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		tok, _ := n.CredentialResolver.TokenForRequest(context.Background())
		if tok != "a" {
			t.Fatalf("expected explicit resolver token, got %q", tok)
		}
	})

	t.Run("defaults surface to hosted scope", func(t *testing.T) {
		n, err := Normalize(Config{CatalogDeps: bundleFactory(nil)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n.DomainScope != HostedDomainScope {
			t.Fatalf("expected HostedDomainScope, got %+v", n.DomainScope)
		}
	})

	t.Run("preserves explicit surface", func(t *testing.T) {
		partial := assembly.DomainScope{Account: true, Websites: true}
		n, err := Normalize(Config{DomainScope: partial, CatalogDeps: bundleFactory(nil)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n.DomainScope != partial {
			t.Fatalf("expected explicit surface, got %+v", n.DomainScope)
		}
	})

	t.Run("nil factory fails validation", func(t *testing.T) {
		if _, err := Normalize(Config{}); err == nil {
			t.Fatal("expected validation error for nil catalog deps")
		}
	})

	t.Run("disagreeing resolvers fail closed", func(t *testing.T) {
		if _, err := Normalize(Config{
			CatalogDeps:        bundleFactory(staticResolver{token: "b"}),
			CredentialResolver: staticResolver{token: "a"},
		}); err == nil {
			t.Fatal("expected conflicting-resolver error")
		}
	})
}
