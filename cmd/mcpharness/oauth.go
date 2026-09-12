package main

// oauth.go implements a minimal, test-only authorization server for the
// harness's `--oauth` mode. It exists purely to satisfy the sunpeak OAuth
// suite (tests/sunpeak/oauth), which drives the full RFC 9728 authorization-
// code + PKCE(S256) flow over raw HTTP against the MCP endpoint:
//
//	discovery -> dynamic client registration -> authorize (secret-as-password)
//	-> token exchange -> protected /mcp call
//
// It is NOT a general OAuth implementation: one shared secret (the password),
// in-memory clients/codes/tokens, short-lived tokens, no refresh. In `--oauth`
// mode /mcp authorizes any issued access token; without one it returns 401
// with a resource-metadata challenge.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

const resourceMetadataKey = "resource_metadata="

type oauthClient struct {
	clientID     string
	redirectURIs []string
}

type oauthAuthCode struct {
	clientID   string
	redirect   string
	challenge  string
	resource   string
	used       bool
	expiresAt  time.Time
}

type oauthAccessToken struct {
	resource  string
	expiresAt time.Time
}

// oauthProvider is the in-memory authorization server for the harness.
type oauthProvider struct {
	mu sync.Mutex

	base   string
	secret string
	// resource is the protected /mcp endpoint URL announced in discovery.
	resource string

	clients map[string]*oauthClient
	codes   map[string]*oauthAuthCode
	tokens  map[string]oauthAccessToken
}

func newOAuthProvider(base, secret string) *oauthProvider {
	return &oauthProvider{
		base:     base,
		secret:   secret,
		resource: base + "/mcp",
		clients:  map[string]*oauthClient{},
		codes:    map[string]*oauthAuthCode{},
		tokens:   map[string]oauthAccessToken{},
	}
}

func randToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// pkceS256 computes the S256 challenge/verifier pairing as the flow expects.
func pkceS256(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// registerHandlers mounts the authorization-server endpoints on mux.
func (o *oauthProvider) registerHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/.well-known/oauth-authorization-server", o.handleMetadata)
	mux.HandleFunc("/.well-known/oauth-protected-resource", o.handleProtectedResource)
	mux.HandleFunc("/oauth/register", o.handleRegister)
	mux.HandleFunc("/oauth/authorize", o.handleAuthorize)
	mux.HandleFunc("/oauth/token", o.handleToken)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (o *oauthProvider) handleMetadata(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                 o.base,
		"authorization_endpoint": o.base + "/oauth/authorize",
		"token_endpoint":         o.base + "/oauth/token",
		"registration_endpoint":  o.base + "/oauth/register",
		"grant_types_supported":  []string{"authorization_code"},
		"response_types_supported": []string{"code"},
		"token_endpoint_auth_methods_supported": []string{"none"},
	})
}

func (o *oauthProvider) handleProtectedResource(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":           o.resource,
		"resource_metadata":  o.base + "/.well-known/oauth-protected-resource",
	})
}

func (o *oauthProvider) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
		return
	}
	var req struct {
		ClientName string   `json:"client_name"`
		Redirects  []string `json:"redirect_uris"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	clientID := "client-" + randToken(8)
	o.mu.Lock()
	o.clients[clientID] = &oauthClient{clientID: clientID, redirectURIs: req.Redirects}
	o.mu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  clientID,
		"client_name":                req.ClientName,
		"redirect_uris":              req.Redirects,
		"token_endpoint_auth_method": "none",
	})
}

func (o *oauthProvider) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	clientID := r.Form.Get("client_id")
	redirect := r.Form.Get("redirect_uri")
	state := r.Form.Get("state")
	if r.Form.Get("password") != o.secret {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_grant", "error_description": "bad password"})
		return
	}
	code := "code-" + randToken(16)
	o.mu.Lock()
	o.codes[code] = &oauthAuthCode{
		clientID:  clientID,
		redirect:  redirect,
		challenge: r.Form.Get("code_challenge"),
		resource:  r.Form.Get("resource"),
		expiresAt: time.Now().Add(5 * time.Minute),
	}
	o.mu.Unlock()

	sep := "?"
	if strings.Contains(redirect, "?") {
		sep = "&"
	}
	loc := redirect + sep + "code=" + code
	if state != "" {
		loc += "&state=" + state
	}
	http.Redirect(w, r, loc, http.StatusFound)
}

func (o *oauthProvider) handleToken(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	if r.Form.Get("grant_type") != "authorization_code" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unsupported_grant_type"})
		return
	}
	code := r.Form.Get("code")
	verifier := r.Form.Get("code_verifier")

	o.mu.Lock()
	c, ok := o.codes[code]
	if ok {
		delete(o.codes, code)
	}
	o.mu.Unlock()
	if !ok || c.used || time.Now().After(c.expiresAt) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant"})
		return
	}
	if c.challenge != "" && pkceS256(verifier) != c.challenge {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant", "error_description": "PKCE verification failed"})
		return
	}

	access := "token-" + randToken(24)
	o.mu.Lock()
	o.tokens[access] = oauthAccessToken{resource: r.Form.Get("resource"), expiresAt: time.Now().Add(10 * time.Minute)}
	o.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": access,
		"token_type":   "Bearer",
		"expires_in":   600,
		"scope":        "",
	})
}

// authorize returns a middleware that accepts only the harness's issued
// access tokens; missing/bad credentials get a 401 with a resource-metadata
// OAuth challenge (mirrors the RFC 9728 protected-resource requirement).
func (o *oauthProvider) authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(authz, "Bearer ") {
			o.challenge(w)
			return
		}
		tok := strings.TrimPrefix(authz, "Bearer ")
		o.mu.Lock()
		t, ok := o.tokens[tok]
		o.mu.Unlock()
		if !ok || time.Now().After(t.expiresAt) {
			o.challenge(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (o *oauthProvider) challenge(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer "+resourceMetadataKey+"\""+o.base+"/.well-known/oauth-protected-resource\"")
	writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized", "error_description": "missing or invalid access token"})
}
