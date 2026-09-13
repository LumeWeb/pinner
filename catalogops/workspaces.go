// Package catalogops implements workspaces domain operations for the operation
// pinner. Each operation drives the core workspaces service directly and
// returns typed data; rendering happens in the frontend wiring layer.
//
// Workspaces are kept deliberately SEPARATE from websites: a workspace is an
// isolated runtime (own portal API key, proxy endpoint, optional website
// association), not a domain-to-CID mapping. Website creation/association
// remains its own domain; the workspace attach relationship (the "publish
// link") is supported where the SDK/API permits it.
package catalogops

import (
	"context"
	"fmt"
	"strconv"

	ipfs "go.lumeweb.com/ipfs-sdk"

	"go.lumeweb.com/opmesh"
	"go.lumeweb.com/pinner/core/config"
	"go.lumeweb.com/pinner/core/workspaces"
)

// WorkspacesDeps are the dependencies the workspaces operations need at
// construction time. All getters are resolved per invocation (never a
// package-init snapshot) so services always use fresh config.
type WorkspacesDeps struct {
	// CfgMgr returns a live config manager for the current invocation. When
	// nil, service() passes nil to the factories (ops then fail on auth if
	// unauthenticated).
	CfgMgr func() config.Manager
	// Secure reports whether to use the secure (HTTPS) endpoint.
	Secure func() bool
	// ServiceFactory builds a workspaces Service. When NewAuthenticated is
	// non-nil and an auth token is available it is used; otherwise
	// ServiceFactory is used.
	ServiceFactory workspaces.ServiceFactoryFunc
	// NewAuthenticated builds an authenticated workspaces service with an
	// explicit auth token; nil means tokens are read from config via
	// ServiceFactory.
	NewAuthenticated func(cfgMgr config.Manager, secure bool, token string) (workspaces.Service, error)
	// GetAuthToken returns an auth token override for the current command
	// context (empty = none).
	GetAuthToken func() string
}

// config returns the live config manager for this invocation, or nil.
func (d WorkspacesDeps) config() config.Manager {
	if d.CfgMgr != nil {
		return d.CfgMgr()
	}
	return nil
}

// service builds the workspaces Service honoring the auth-token override. The
// per-invocation --auth-token flag (threaded through input) takes precedence
// over the deps.GetAuthToken() config fallback (flag over config).
func (d WorkspacesDeps) service(input map[string]any) (workspaces.Service, error) {
	cfgMgr := d.config()
	if cfgMgr == nil {
		return nil, fmt.Errorf("catalogops: no config manager available")
	}
	secure := false
	if d.Secure != nil {
		secure = d.Secure()
	}
	if d.NewAuthenticated != nil && d.GetAuthToken != nil {
		if t := authTokenFromInput(input); t != "" {
			return d.NewAuthenticated(cfgMgr, secure, t)
		}
		if t := d.GetAuthToken(); t != "" {
			return d.NewAuthenticated(cfgMgr, secure, t)
		}
	}
	return d.ServiceFactory(cfgMgr, secure), nil
}

// WorkspacesOperations returns the catalog operations for the workspaces
// domain (the `workspaces` subcommand group), each driving the core
// workspaces.Service.
//
// Resolve (runtime workspace self-identification) is deliberately NOT exposed
// here: it is a runtime-internal concern driven by the Coolify-injected
// resource UUID, not a normal user CLI/MCP operation.
func WorkspacesOperations(d WorkspacesDeps) []opmesh.Operation {
	return []opmesh.Operation{
		workspacesList(d),
		workspacesGet(d),
		workspacesCreate(d),
		workspacesAttach(d),
		workspacesSuspend(d),
		workspacesResume(d),
		workspacesAccess(d),
		workspacesDelete(d),
	}
}

// workspaceIDArg is the shared `id` positional/argument descriptor for
// operations that select a single workspace by its numeric ID. It is a
// flexible ID so either the integer or decimal-string form is accepted.
func workspaceIDArg(help string) opmesh.OperationArg {
	return opmesh.OperationArg{
		Name: "id", Type: opmesh.ArgTypeFlexibleID, Required: true, Help: help,
	}
}

// requireWorkspaceID reads the required `id` (flexible string-or-integer) and
// validates it is present.
func requireWorkspaceID(input map[string]any) (string, error) {
	id := opmesh.StrFlexibleArg(input, "id", "")
	if id == "" {
		return "", fmt.Errorf("workspace id is required")
	}
	return id, nil
}

// workspaceIDToInt parses a flexible workspace/website id into an int. It is
// used by operations whose SDK call needs a numeric id (e.g. attach).
func workspaceIDToInt(input map[string]any, key string) (int, error) {
	s := opmesh.StrFlexibleArg(input, key, "")
	if s == "" {
		return 0, fmt.Errorf("%s is required", key)
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: must be a numeric id", key, s)
	}
	return n, nil
}

// workspacesList is the `workspaces list` operation. Returns []ipfs.WorkspaceResponse.
func workspacesList(d WorkspacesDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "workspaces_list",
		Title:       "List workspaces",
		Summary:     "List all workspaces",
		Description: "List all workspaces for the authenticated user, including unattached workspaces. Each row carries the workspace ID, domain, label, status, and its attached website ID (blank when unattached).",
		Category:    "core",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "",
		Args:        opmesh.ListArgs(),
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, svcErr := d.service(input)
			if svcErr != nil {
				return nil, svcErr
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}

			page := opmesh.ParseListPage(input, 10)
			opts := workspaces.ListOptions{
				Start: page.Start,
				Limit: page.Limit,
			}
			workspaceList, err := svc.List(ctx, opts)
			if err != nil {
				return nil, err
			}

			headers := []string{"ID", "DOMAIN", "LABEL", "STATUS", "WEBSITE ID", "CREATED"}
			rows := make([][]string, 0, len(workspaceList))
			for _, w := range workspaceList {
				websiteID := "-"
				if w.WebsiteId != nil {
					websiteID = fmt.Sprintf("%d", *w.WebsiteId)
				}
				rows = append(rows, []string{
					fmt.Sprintf("%d", w.Id), w.Domain, w.Label, w.Status, websiteID,
					w.Created.Format("2006-01-02 15:04:05"),
				})
			}
			return NewListResult(workspaceList, ListResultMeta{
				Noun:    "workspace(s)",
				Headers: headers,
				Rows:    rows,
			}), nil
		}),
	})
}

// workspacesGet is the `workspaces get` operation. Returns
// *ipfs.WorkspaceResponse.
func workspacesGet(d WorkspacesDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "workspaces_get",
		Title:       "Get workspace details",
		Summary:     "Get full details of one workspace",
		Description: "Get full details of one workspace by its numeric ID: ID, domain, label, status, created/updated timestamps, any error, and its attached website ID (blank/nil when unattached).",
		Category:    "core",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<id>",
		Args: []opmesh.OperationArg{
			workspaceIDArg("Numeric ID of the workspace to get. Required."),
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, svcErr := d.service(input)
			if svcErr != nil {
				return nil, svcErr
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id, err := requireWorkspaceID(input)
			if err != nil {
				return nil, err
			}
			return svc.Get(ctx, id)
		}),
	})
}

// workspacesCreate is the `workspaces create` operation. Returns
// *ipfs.WorkspaceResponse. A workspace may be created unattached (no
// website-id) or immediately attached to a website the user owns (the publish
// link) by passing --website-id.
func workspacesCreate(d WorkspacesDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "workspaces_create",
		Title:       "Create a workspace",
		Summary:     "Create a new workspace",
		Description: "Create a new isolated workspace with its own portal API key and proxy endpoint. Pass the optional website-id to attach it to a website the user owns (the publish link); omit it to create an unattached workspace that needs no Website record or domain.",
		Category:    "core",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "",
		Args: []opmesh.OperationArg{
			{Name: "website-id", Type: opmesh.ArgTypeFlexibleID, Required: false, Help: "Optional numeric ID of a website you own to attach the workspace to (the publish link)"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, svcErr := d.service(input)
			if svcErr != nil {
				return nil, svcErr
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			req := ipfs.WorkspaceRequest{}
			if s := opmesh.StrFlexibleArg(input, "website-id", ""); s != "" {
				websiteID, err := strconv.Atoi(s)
				if err != nil {
					return nil, fmt.Errorf("invalid website-id %q: must be a numeric id", s)
				}
				req.WebsiteId = &websiteID
			}
			return svc.Create(ctx, req)
		}),
	})
}

// workspacesAttach is the `workspaces attach` operation. Returns
// *ipfs.WorkspaceResponse.
func workspacesAttach(d WorkspacesDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "workspaces_attach",
		Title:       "Attach a workspace to a website",
		Summary:     "Attach a workspace to a website",
		Description: "Attach a workspace to a website you own (the publish link), by the workspace's numeric ID and the target website's numeric ID. Returns the updated workspace carrying the new website association.",
		Category:    "core",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<id>",
		Args: []opmesh.OperationArg{
			workspaceIDArg("Numeric ID of the workspace to attach. Required."),
			{Name: "website-id", Type: opmesh.ArgTypeFlexibleID, Required: true, Help: "Numeric ID of the website you own to attach the workspace to. Required."},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, svcErr := d.service(input)
			if svcErr != nil {
				return nil, svcErr
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id, err := requireWorkspaceID(input)
			if err != nil {
				return nil, err
			}
			websiteID, err := workspaceIDToInt(input, "website-id")
			if err != nil {
				return nil, err
			}
			return svc.Attach(ctx, id, websiteID)
		}),
	})
}

// workspacesSuspend is the `workspaces suspend` operation. Returns
// *ipfs.WorkspaceResponse. Suspension is reversible via resume; it is a
// mutate (not destructive) operation.
func workspacesSuspend(d WorkspacesDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "workspaces_suspend",
		Title:       "Suspend a workspace",
		Summary:     "Suspend a workspace",
		Description: "Suspend a workspace by its numeric ID, stopping its runtime. Suspension is reversible with workspaces_resume.",
		Category:    "core",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<id>",
		Args: []opmesh.OperationArg{
			workspaceIDArg("Numeric ID of the workspace to suspend. Required."),
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, svcErr := d.service(input)
			if svcErr != nil {
				return nil, svcErr
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id, err := requireWorkspaceID(input)
			if err != nil {
				return nil, err
			}
			return svc.Suspend(ctx, id)
		}),
	})
}

// workspacesResume is the `workspaces resume` operation. Returns
// *ipfs.WorkspaceResponse.
func workspacesResume(d WorkspacesDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "workspaces_resume",
		Title:       "Resume a workspace",
		Summary:     "Resume a suspended workspace",
		Description: "Resume a suspended workspace by its numeric ID, restarting its runtime.",
		Category:    "core",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<id>",
		Args: []opmesh.OperationArg{
			workspaceIDArg("Numeric ID of the workspace to resume. Required."),
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, svcErr := d.service(input)
			if svcErr != nil {
				return nil, svcErr
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id, err := requireWorkspaceID(input)
			if err != nil {
				return nil, err
			}
			return svc.Resume(ctx, id)
		}),
	})
}

// workspacesAccess is the `workspaces access` operation. Returns
// *ipfs.WorkspaceAccessResponse (the owner's proxy Basic Auth credentials).
//
// These credentials are SENSITIVE. The operation is InteractionHumanOnly so a
// model actor is refused (ErrHumanRequired -> needs_human hand-off) by the
// opmesh Invoke gate before the handler ever runs, while remaining discoverable
// (VisibilityBoth) so it surfaces in search/describe. Only a human invokes it.
func workspacesAccess(d WorkspacesDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "workspaces_access",
		Title:       "Get workspace access credentials",
		Summary:     "Get a workspace's proxy access credentials",
		Description: "Get the owner's proxy Basic Auth credentials (username/password) for a workspace by its numeric ID. These are sensitive credentials — this operation requires a human to run (model agents are handed off). Pass rotate=true to rotate the proxy credential before returning.",
		Category:    "core",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionHumanOnly,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<id>",
		Args: []opmesh.OperationArg{
			workspaceIDArg("Numeric ID of the workspace to get access credentials for. Required."),
			{Name: "rotate", Type: opmesh.ArgTypeBool, Required: false, Help: "Rotate the proxy credential before returning (single attempt; the existing credential is replaced)"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, svcErr := d.service(input)
			if svcErr != nil {
				return nil, svcErr
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id, err := requireWorkspaceID(input)
			if err != nil {
				return nil, err
			}
			rotate := opmesh.BoolArg(input, "rotate", false)
			return svc.Access(ctx, id, rotate)
		}),
	})
}

// WorkspaceDeleteResult is the typed data returned by the delete operation so
// the frontend can render the deleted workspace's identifier and final state.
type WorkspaceDeleteResult struct {
	ID     string `json:"id"`
	Status string `json:"status,omitempty"`
}

// workspacesDelete is the `workspaces delete` operation. DESTRUCTIVE. The
// core Delete returns the workspace's final state (marked deleting, portal API
// key revoked, backing application removed); the handler returns a
// WorkspaceDeleteResult so the frontend can render a confirmation. confirm is
// marked AgentConfirm so a model actor that explicitly passes confirm=true runs
// it headlessly; otherwise the destructive gate surfaces a confirm hand-off.
func workspacesDelete(d WorkspacesDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "workspaces_delete",
		Title:       "Delete a workspace",
		Summary:     "Delete a workspace",
		Description: "Delete a workspace by its numeric ID. DESTRUCTIVE and irreversible: the workspace is marked deleting, its portal API key revoked, and its backing application removed. There is no undo. Requires confirm=true.",
		Category:    "core",
		Safety:      opmesh.SafetyDestructive,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<id>",
		Args: []opmesh.OperationArg{
			workspaceIDArg("Numeric ID of the workspace to delete. Required."),
			{Name: "confirm", Type: opmesh.ArgTypeBool, Required: true, AgentConfirm: true, Help: "Confirm the destructive delete"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			if !opmesh.BoolArg(input, "confirm", false) {
				return nil, fmt.Errorf("workspaces_delete: confirmation is required to delete the workspace")
			}
			svc, svcErr := d.service(input)
			if svcErr != nil {
				return nil, svcErr
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id, err := requireWorkspaceID(input)
			if err != nil {
				return nil, err
			}
			result, err := svc.Delete(ctx, id)
			if err != nil {
				return nil, err
			}
			res := &WorkspaceDeleteResult{ID: id}
			if result != nil {
				res.Status = result.Status
			}
			if res.Status == "" {
				res.Status = "deleting"
			}
			return res, nil
		}),
	})
}
