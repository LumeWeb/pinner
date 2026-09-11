package hosted

import (
	"context"
	"reflect"
	"testing"

	"go.lumeweb.com/opmesh"

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

	t.Run("pre-assembled catalog satisfies validation without a factory", func(t *testing.T) {
		cat := opmesh.NewCatalog()
		if err := (Config{Catalog: cat}).Validate(); err != nil {
			t.Fatalf("expected pre-assembled Catalog to validate without CatalogDeps, got %v", err)
		}
	})

	t.Run("typed-nil catalog fails validation without a factory", func(t *testing.T) {
		// A typed-nil Catalog (interface holding a nil concrete value) reads as
		// UNSET, so it must not act as a usable catalog source on its own.
		concrete := opmesh.NewCatalog()
		typedNil := reflect.Zero(reflect.TypeOf(concrete)).Interface().(opmesh.Catalog)
		if err := (Config{Catalog: typedNil}).Validate(); err == nil {
			t.Fatal("expected typed-nil Catalog to fail validation (no CatalogDeps)")
		}
	})

	t.Run("normalize carries a pre-assembled catalog", func(t *testing.T) {
		cat := opmesh.NewCatalog()
		n, err := Normalize(Config{Catalog: cat})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n.Catalog != cat {
			t.Fatal("expected Normalized.Catalog to carry the pre-assembled catalog")
		}
		if n.CatalogDeps != nil {
			t.Fatal("expected Normalized.CatalogDeps to be nil when a pre-assembled Catalog is set")
		}
	})

	t.Run("factory wins over a typed-nil catalog", func(t *testing.T) {
		// A typed-nil Catalog reads as UNSET, so the CatalogDeps path must
		// apply and define the catalog source for the normalized plan.
		concrete := opmesh.NewCatalog()
		typedNil := reflect.Zero(reflect.TypeOf(concrete)).Interface().(opmesh.Catalog)
		n, err := Normalize(Config{
			Catalog:     typedNil,
			CatalogDeps: bundleFactory(nil),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n.CatalogDeps == nil {
			t.Fatal("expected CatalogDeps to be preserved when Catalog is typed-nil")
		}
		if !isNilCatalog(n.Catalog) {
			t.Fatal("expected typed-nil Catalog to be carried as unset")
		}
	})

	t.Run("pre-assembled catalog with explicit resolver keeps explicit resolver", func(t *testing.T) {
		cat := opmesh.NewCatalog()
		explicit := staticResolver{token: "a"}
		n, err := Normalize(Config{Catalog: cat, CredentialResolver: explicit})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		tok, _ := n.CredentialResolver.TokenForRequest(context.Background())
		if tok != "a" {
			t.Fatalf("expected explicit resolver token, got %q", tok)
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
