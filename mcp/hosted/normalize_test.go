package hosted

import (
	"context"
	"errors"
	"testing"

	"go.lumeweb.com/pinner/assembly"
)

type staticResolver struct{ token string }

func (r staticResolver) TokenForRequest(context.Context) (string, error) {
	if r.token == "" {
		return "", errors.New("not authenticated")
	}
	return r.token, nil
}

type identifiableResolver struct{ id string }

func (r identifiableResolver) ResolverIdentity() string { return r.id }
func (r identifiableResolver) TokenForRequest(context.Context) (string, error) {
	return "token:" + r.id, nil
}

func TestNormalizeCredentialResolvers(t *testing.T) {
	t.Run("nil explicit defers to bundle resolver", func(t *testing.T) {
		got, err := NormalizeCredentialResolvers(nil, []CredentialResolver{staticResolver{token: "b"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		tok, _ := got.TokenForRequest(context.Background())
		if tok != "b" {
			t.Fatalf("expected bundle resolver token, got %q", tok)
		}
	})

	t.Run("nil bundle keeps explicit resolver", func(t *testing.T) {
		explicit := staticResolver{token: "a"}
		got, err := NormalizeCredentialResolvers(explicit, []CredentialResolver{nil})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		tok, _ := got.TokenForRequest(context.Background())
		if tok != "a" {
			t.Fatalf("expected explicit resolver token, got %q", tok)
		}
	})

	t.Run("identical value resolvers accepted", func(t *testing.T) {
		explicit := staticResolver{token: "same"}
		got, err := NormalizeCredentialResolvers(explicit, []CredentialResolver{staticResolver{token: "same"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		tok, _ := got.TokenForRequest(context.Background())
		if tok != "same" {
			t.Fatalf("expected explicit resolver token, got %q", tok)
		}
	})

	t.Run("value-different resolvers fail closed", func(t *testing.T) {
		explicit := staticResolver{token: "a"}
		if _, err := NormalizeCredentialResolvers(explicit, []CredentialResolver{staticResolver{token: "b"}}); err == nil {
			t.Fatal("expected conflicting-resolver error")
		}
	})

	t.Run("shared identity accepted for closure-backed resolvers", func(t *testing.T) {
		explicit := identifiableResolver{id: "user-scoped"}
		got, err := NormalizeCredentialResolvers(explicit, []CredentialResolver{identifiableResolver{id: "user-scoped"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		tok, _ := got.TokenForRequest(context.Background())
		if tok != "token:user-scoped" {
			t.Fatalf("expected explicit resolver token, got %q", tok)
		}
	})

	t.Run("disagreeing identities fail closed", func(t *testing.T) {
		explicit := identifiableResolver{id: "a"}
		if _, err := NormalizeCredentialResolvers(explicit, []CredentialResolver{identifiableResolver{id: "b"}}); err == nil {
			t.Fatal("expected conflicting-resolver error")
		}
	})

	t.Run("non-uniform bundle sampling converges to non-nil", func(t *testing.T) {
		// A factory that returns nil on its first invocation and a resolver
		// afterwards must not leave a boundary resolver-less.
		got, err := NormalizeCredentialResolvers(nil, []CredentialResolver{nil, staticResolver{token: "late"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		tok, _ := got.TokenForRequest(context.Background())
		if tok != "late" {
			t.Fatalf("expected late resolver token, got %q", tok)
		}
	})
}

func TestConfigValidate(t *testing.T) {
	// A hosted construction requires a catalog-deps factory: the catalog is the
	// source of the compiled tool surface, so a nil factory is a wiring bug.
	if err := (Config{}).Validate(); err == nil {
		t.Fatal("expected zero Config to fail validation (no catalog deps)")
	}
	ok := Config{CatalogDeps: func() *assembly.CatalogDepsBundle { return nil }}
	if err := ok.Validate(); err != nil {
		t.Fatalf("expected valid Config, got %v", err)
	}
}

func TestConfigHostedDomainScope(t *testing.T) {
	// A zero DomainScope defaults to the restricted hosted surface, never the
	// implicit full surface (which would admit the Sia vault and portal admin).
	got := (Config{}).HostedDomainScope()
	if got != HostedDomainScope {
		t.Fatalf("expected zero Config to default to HostedDomainScope, got %+v", got)
	}
	partial := assembly.DomainScope{Account: true}
	if got := (Config{DomainScope: partial}).HostedDomainScope(); got != partial {
		t.Fatalf("expected explicit DomainScope to be preserved, got %+v", got)
	}
}
