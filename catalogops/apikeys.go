// Package catalogops implements API-key domain operations for the operation
// catalog, driving the core apikeys service directly.
package catalogops

import (
	"context"
	"fmt"

	portalsdk "go.lumeweb.com/portal-sdk"

	"go.lumeweb.com/opmesh"
	"go.lumeweb.com/pinner/core/apikeys"
)

// APIKeyDrop is the out-of-band one-time hand-off seam for freshly created
// API-key values. Implementations hold the secret in memory only, until the
// human opens the returned URL once; the agent/MCP tool channel never carries
// the value. This is the api-keys analogue of the vault setup's OOB
// coordinators (vault_setup.go): catalogops deposits the secret through this
// seam and returns only typed hand-off data, so no per-frontend parsing or a
// second secret store is needed — composition roots wire the same one-time
// drop machinery they already run for vault seeds.
type APIKeyDrop interface {
	// Drop stores the key value for one-time human retrieval and returns the
	// HTTPS URL the human opens to view it. The value must never be served
	// back through any agent tool channel, and the store must expire the
	// entry after first retrieval.
	Drop(ctx context.Context, key *portalsdk.APIKey) (dropURL string, err error)
}

// APIKeysDeps injects the dependencies for building an apikeys.Service.
type APIKeysDeps struct {
	// Service returns a live apikeys.Service for the current invocation,
	// honoring the per-invocation auth-token override in the input map.
	Service func(input map[string]any) apikeys.Service

	// OOBKeyDropBuild returns the OOB drop coordinator for the current
	// invocation, or nil when this assembly is the human-at-terminal CLI
	// surface (where returning the key value once in process stdout is the
	// delivery). Every model-facing assembly — local stdio MCP and hosted
	// MCP alike — MUST wire this to its one-time in-memory drop store; the
	// policy contract forbids the key value on an agent channel, so an MCP
	// assembly without a wired drop must not advertise api_keys_create.
	// Handlers resolve the coordinator per invocation so live config/token
	// changes are honored, matching the lazy-deps pattern used throughout.
	OOBKeyDropBuild func(input map[string]any) APIKeyDrop
}

// APIKeysOperations returns the catalog operations for the api-keys domain
// (list, create, delete).
func APIKeysOperations(d APIKeysDeps) []opmesh.Operation {
	return []opmesh.Operation{
		apiKeysList(d),
		apiKeysCreate(d),
		apiKeysDelete(d),
	}
}

func apiKeysList(d APIKeysDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name: "api_keys_list", Title: "List API keys", Summary: "List all API keys",
		Description: "List all API keys for your account, optionally filtered by name.",
		Category:    "account", Safety: opmesh.SafetyRead, Interaction: opmesh.InteractionAgentSafe, Visibility: opmesh.VisibilityBoth,
		Positional: "",
		Args: append(opmesh.ListArgs(),
			opmesh.OperationArg{Name: "search", Type: opmesh.ArgTypeString, Help: "Full-text search evaluated server-side against key name"},
		),
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc := d.Service(input)
			if svc == nil {
				return nil, fmt.Errorf("api-keys service unavailable")
			}
			keys, _, err := svc.ListAPIKeys(ctx, opmesh.SearchArg(input))
			if err != nil {
				return nil, err
			}
			page := opmesh.ParseList(input)
			items := slicePage(keys, page.Start, page.Limit)
			headers := []string{"UUID", "NAME"}
			rows := make([][]string, 0, len(items))
			for _, k := range items {
				if k == nil {
					continue
				}
				rows = append(rows, []string{k.Uuid.String(), k.Name})
			}
			return NewListResult(items, ListResultMeta{
				Noun: "API key(s)", Headers: headers, Rows: rows,
			}), nil
		}),
	})
}

// APIKeyCreateResult is the data returned by api_keys_create. The freshly
// created key value is delivered exactly once on the channel the assembly
// declared at wiring time: `token` is set only on the human-at-terminal CLI
// surface (OOBKeyDropBuild nil), `drop_url` only when the value was handed to
// a one-time out-of-band drop. A model-facing channel never sees `token`.
type APIKeyCreateResult struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Token   string `json:"token,omitempty"`    // CLI human-terminal delivery only
	DropURL string `json:"drop_url,omitempty"` // one-time OOB retrieval only
	Message string `json:"message,omitempty"`
}

// oobKeyDrop resolves the OOB drop coordinator for this invocation, or nil
// when the assembly did not wire one (human-at-terminal CLI surface).
func (d APIKeysDeps) oobKeyDrop(input map[string]any) APIKeyDrop {
	if d.OOBKeyDropBuild == nil {
		return nil
	}
	return d.OOBKeyDropBuild(input)
}

func apiKeysCreate(d APIKeysDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name: "api_keys_create", Title: "Create an API key", Summary: "Create a new API key",
		Description: "Create a new API key for your account. The key value is delivered exactly once and is not restorable: it is printed to your terminal once and can never be retrieved again. If a key is exposed, delete it via api_keys_delete and create a new one.",
		Category:    "account", Safety: opmesh.SafetyMutate, Interaction: opmesh.InteractionAgentSafe, Visibility: opmesh.VisibilityBoth,
		Positional: "<name>",
		Args: []opmesh.OperationArg{
			{Name: "name", Type: opmesh.ArgTypeString, Required: true, Help: "Key name"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc := d.Service(input)
			if svc == nil {
				return nil, fmt.Errorf("api-keys service unavailable")
			}
			name := opmesh.StrArg(input, "name", "")
			if name == "" {
				return nil, fmt.Errorf("api_keys_create: key name is required")
			}
			key, err := svc.CreateAPIKey(ctx, name)
			if err != nil {
				return nil, err
			}
			if key == nil {
				return nil, fmt.Errorf("api_keys_create: empty key returned for %q", name)
			}
			out := &APIKeyCreateResult{UUID: key.Uuid.String(), Name: key.Name}
			if drop := d.oobKeyDrop(input); drop != nil {
				dropURL, derr := drop.Drop(ctx, key)
				if derr != nil {
					return nil, fmt.Errorf("api_keys_create: %w", derr)
				}
				out.DropURL = dropURL
				out.Message = "Key created. Open drop_url once to view the key value: it is held in memory only until first retrieval and is shown exactly once."
				return out, nil
			}
			out.Token = key.Token
			out.Message = "Key created. The key value is shown exactly once and cannot be retrieved again."
			return out, nil
		}),
	})
}

func apiKeysDelete(d APIKeysDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name: "api_keys_delete", Title: "Delete an API key", Summary: "Delete an API key",
		Description: "Delete an API key by name or UUID. DESTRUCTIVE: the key is revoked immediately — every access made with it stops working right away — and it cannot be recovered or reused; the only remediation is creating a replacement with api_keys_create. Use --force to delete the key currently used for authentication; deleting any other key needs no extra flag.",
		Category:    "account", Safety: opmesh.SafetyDestructive, Interaction: opmesh.InteractionAgentSafe, Visibility: opmesh.VisibilityBoth,
		Positional: "<id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeString, Required: true, Help: "API key name or UUID"},
			{Name: "confirm", Type: opmesh.ArgTypeBool, Default: "false", Help: "Allow deleting the key currently used for authentication"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc := d.Service(input)
			if svc == nil {
				return nil, fmt.Errorf("api-keys service unavailable")
			}
			id := opmesh.StrArg(input, "id", "")
			if id == "" {
				return nil, fmt.Errorf("api_keys_delete: key name or UUID is required")
			}
			// The confirm arg is the CLI's self-delete override (--force): the
			// core service blocks deleting the key currently authenticating
			// the caller unless it is set, and is the sole judge of that
			// (no handler-level friction here — deletes of OTHER keys must
			// keep running without it). Agent (MCP) surfaces never reach the
			// handler unconfirmed: Invoke refuses destructive ops for a model
			// actor, so every MCP deletion goes through the human hand-off.
			force := opmesh.BoolArg(input, "confirm", false)
			if err := svc.DeleteAPIKey(ctx, id, force); err != nil {
				return nil, err
			}
			return &APIKeyDeleteResult{ID: id}, nil
		}),
	})
}

// APIKeysListResult wraps the raw []*portalsdk.APIKey + total from the core
// service so the frontend can render a typed result.
type APIKeysListResult struct {
	Keys  []*portalsdk.APIKey `json:"keys"`
	Total int                 `json:"total"`
}

// APIKeyDeleteResult reports a successful API key deletion (or a self-delete
// that was explicitly forced).
type APIKeyDeleteResult struct {
	ID      string `json:"id"`
	Message string `json:"message,omitempty"`
}
