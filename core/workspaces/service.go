// Package workspaces provides workspace operations for the Pinner
// content-network services. It is defined by the Service interface and is
// Output-free.
//
// A workspace is an isolated, self-contained application/runtime that can be
// published through the portal. Whereas a website associates a domain with a
// CID, a workspace is a separate frontend/backend concept: it owns a portal
// API key, a proxy endpoint (with Basic Auth credentials), and an optional
// association to a website (the "publish link") the user owns. Workspaces may
// exist unattached — needing no Website record or domain.
package workspaces

import (
	"context"

	ipfs "go.lumeweb.com/ipfs-sdk"
	"go.lumeweb.com/pinner"
	"go.lumeweb.com/pinner/core/config"
	coreerrors "go.lumeweb.com/pinner/core/errors"
	"go.lumeweb.com/pinner/core/ipfsbase"
	"go.uber.org/zap"
)

// Service defines the interface for workspace operations. Resolve (runtime
// workspace self-identification) is intentionally NOT exposed here: it is a
// runtime-internal concern driven by the Coolify-injected resource UUID, not a
// normal user CLI/MCP operation.
type Service interface {
	RequireAuthenticated() error
	// SetAuthToken hot-updates the auth token on a running service without
	// reconstructing it (used by long-lived consumers on config live-reload).
	SetAuthToken(token string)
	// List retrieves workspaces owned by the authenticated user, including
	// unattached workspaces, applying the shared paging options.
	List(ctx context.Context, opts ListOptions) ([]ipfs.WorkspaceResponse, error)
	// Get retrieves a specific workspace by its numeric ID.
	Get(ctx context.Context, id string) (*ipfs.WorkspaceResponse, error)
	// Create creates a workspace. A non-nil website_id in req attaches it to a
	// website the user owns (the publish link); a nil website_id is omitted,
	// creating an unattached workspace.
	Create(ctx context.Context, req ipfs.WorkspaceRequest) (*ipfs.WorkspaceResponse, error)
	// Delete deletes a workspace and returns its final state. The workspace is
	// marked deleting, its portal API key revoked, and the backing application
	// removed.
	Delete(ctx context.Context, id string) (*ipfs.WorkspaceResponse, error)
	// Access returns the owner's proxy Basic Auth credentials for a workspace.
	// rotate rotates the proxy credential before returning. These credentials
	// are sensitive and must not be casually exposed.
	Access(ctx context.Context, id string, rotate bool) (*ipfs.WorkspaceAccessResponse, error)
	// Attach attaches a workspace to a website the user owns (the publish
	// link), selected by its numeric website ID.
	Attach(ctx context.Context, id string, websiteID int) (*ipfs.WorkspaceResponse, error)
	// Resume resumes a suspended workspace.
	Resume(ctx context.Context, id string) (*ipfs.WorkspaceResponse, error)
	// Suspend suspends a workspace.
	Suspend(ctx context.Context, id string) (*ipfs.WorkspaceResponse, error)
}

// ListOptions is the workspaces paging options: it aliases the shared generic
// pinner.ListOptions with an empty filter struct, so paging is common to every
// service. The workspaces list endpoint has no client-side filter fields.
type ListOptions = pinner.ListOptions[struct{}]

// WorkspaceSDKOpts translates ListOptions into the ipfs-sdk list options. Only
// non-zero fields are emitted, so an empty ListOptions produces no query
// mutation at all (the server applies its default list window).
func WorkspaceSDKOpts(o ListOptions) []ipfs.ListWorkspacesOption {
	var opts []ipfs.ListWorkspacesOption
	if o.Start > 0 {
		opts = append(opts, ipfs.WithWorkspacesStart(o.Start))
	}
	if o.Limit > 0 {
		opts = append(opts, ipfs.WithWorkspacesLimit(o.Limit))
	}
	return opts
}

// service implements the Service interface using the ipfs.WorkspacesService.
type service struct {
	*ipfsbase.Base
	ws     ipfs.WorkspacesService
	client *ipfs.Client
	log    *zap.Logger
}

// Option is a function that configures a service.
type Option func(*service)

// WithAuthToken sets an auth token override that takes precedence over config.
func WithAuthToken(token string) Option {
	return func(s *service) {
		s.Base.SetAuthTokenOverride(token)
	}
}

// WithClient sets a pre-configured ipfs.Client, bypassing the default
// ipfs.NewClient() call.
func WithClient(client *ipfs.Client) Option {
	return func(s *service) {
		s.client = client
	}
}

// New creates a new workspaces Service instance.
// It must NOT copy cfgMgr.Config().AuthToken into the base auth token:
// leaving it empty lets GetAuthToken() read config live at request time, so a
// long-lived service live-reloads a `pinner login` that rewrites the on-disk
// token. Explicit WithAuthToken overrides still pin a token and take
// precedence. A nil logger is treated as a no-op logger.
func New(cfgMgr config.Manager, apiEndpoint string, logger *zap.Logger, opts ...Option) Service {
	s := &service{
		Base: ipfsbase.New(cfgMgr),
		log:  logger,
	}
	if s.log == nil {
		s.log = zap.NewNop()
	}
	for _, opt := range opts {
		opt(s)
	}

	if s.client != nil {
		s.ws = s.client.Workspaces()
	} else {
		client, err := ipfs.NewClient(apiEndpoint, s.GetAuthToken())
		if err != nil {
			s.log.Debug("could not create workspaces client", zap.Error(err))
			s.ws = nil
			return s
		}
		s.client = client
		s.ws = client.Workspaces()
	}
	return s
}

// SetAuthToken hot-updates the auth token on the retained *ipfs.Client and
// re-fetches the sub-service so a running service reflects a config token
// change without being reconstructed. No-op when no client is retained.
// The write lock serializes this (config-watcher goroutine) with request reads.
func (s *service) SetAuthToken(token string) {
	s.Lock()
	defer s.Unlock()
	if s.client != nil {
		if err := s.client.SetAuthToken(token); err == nil {
			s.ws = s.client.Workspaces()
		}
	}
}

// requireService returns the current sub-service under the read lock, so the
// config-watcher goroutine (SetAuthToken) cannot swap s.ws mid-request.
func (s *service) requireService() (ipfs.WorkspacesService, error) {
	s.RLock()
	defer s.RUnlock()
	if s.ws == nil {
		return nil, coreerrors.ErrServiceUnavailable
	}
	return s.ws, nil
}

// List retrieves the workspaces for the authenticated user, including
// unattached workspaces, applying the shared paging options.
func (s *service) List(ctx context.Context, opts ListOptions) ([]ipfs.WorkspaceResponse, error) {
	if err := s.RequireAuthenticated(); err != nil {
		return nil, err
	}
	svc, err := s.requireService()
	if err != nil {
		return nil, err
	}
	return svc.List(ctx, WorkspaceSDKOpts(opts)...)
}

// Get retrieves a specific workspace by its numeric ID.
func (s *service) Get(ctx context.Context, id string) (*ipfs.WorkspaceResponse, error) {
	if err := s.RequireAuthenticated(); err != nil {
		return nil, err
	}
	svc, err := s.requireService()
	if err != nil {
		return nil, err
	}
	return svc.Get(ctx, id)
}

// Create creates a workspace. A non-nil website_id in req attaches it to a
// website the user owns (the publish link); a nil website_id is omitted,
// creating an unattached workspace.
func (s *service) Create(ctx context.Context, req ipfs.WorkspaceRequest) (*ipfs.WorkspaceResponse, error) {
	if err := s.RequireAuthenticated(); err != nil {
		return nil, err
	}
	svc, err := s.requireService()
	if err != nil {
		return nil, err
	}
	return svc.Create(ctx, req)
}

// Delete deletes a workspace and returns its final state.
func (s *service) Delete(ctx context.Context, id string) (*ipfs.WorkspaceResponse, error) {
	if err := s.RequireAuthenticated(); err != nil {
		return nil, err
	}
	svc, err := s.requireService()
	if err != nil {
		return nil, err
	}
	return svc.Delete(ctx, id)
}

// Access returns the owner's proxy Basic Auth credentials for a workspace.
// rotate rotates the proxy credential before returning.
func (s *service) Access(ctx context.Context, id string, rotate bool) (*ipfs.WorkspaceAccessResponse, error) {
	if err := s.RequireAuthenticated(); err != nil {
		return nil, err
	}
	svc, err := s.requireService()
	if err != nil {
		return nil, err
	}
	return svc.Access(ctx, id, rotate)
}

// Attach attaches a workspace to a website the user owns (the publish link).
func (s *service) Attach(ctx context.Context, id string, websiteID int) (*ipfs.WorkspaceResponse, error) {
	if err := s.RequireAuthenticated(); err != nil {
		return nil, err
	}
	svc, err := s.requireService()
	if err != nil {
		return nil, err
	}
	return svc.Attach(ctx, id, websiteID)
}

// Resume resumes a suspended workspace.
func (s *service) Resume(ctx context.Context, id string) (*ipfs.WorkspaceResponse, error) {
	if err := s.RequireAuthenticated(); err != nil {
		return nil, err
	}
	svc, err := s.requireService()
	if err != nil {
		return nil, err
	}
	return svc.Resume(ctx, id)
}

// Suspend suspends a workspace.
func (s *service) Suspend(ctx context.Context, id string) (*ipfs.WorkspaceResponse, error) {
	if err := s.RequireAuthenticated(); err != nil {
		return nil, err
	}
	svc, err := s.requireService()
	if err != nil {
		return nil, err
	}
	return svc.Suspend(ctx, id)
}
