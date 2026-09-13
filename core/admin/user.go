package admin

import (
	"context"
	"sync"

	"go.lumeweb.com/pinner/core/config"
	"go.lumeweb.com/portal-sdk/admin"
)

// userAdminService implements the UserAdminService interface using the
// admin.UserService (admin user CRUD against the portal admin API).
type userAdminService struct {
	mu      sync.RWMutex
	base    *adminServiceBase
	service *admin.UserService
}

// UserAdminServiceFactory creates a UserAdminService with dependencies.
type UserAdminServiceFactory func(cfgMgr config.Manager) UserAdminService

// DefaultUserAdminServiceFactory creates a default UserAdminService instance.
func DefaultUserAdminServiceFactory(cfgMgr config.Manager) UserAdminService {
	return NewUserAdminService(cfgMgr, cfgMgr.Config().GetAdminEndpoint())
}

// NewUserAdminService creates a new UserAdminService instance.
func NewUserAdminService(cfgMgr config.Manager, apiEndpoint string) UserAdminService {
	return &userAdminService{
		base: newAdminServiceBase(cfgMgr, apiEndpoint),
	}
}

// UserAdminService defines the interface for admin user operations (managing
// portal accounts through the admin API). Password is always hashed
// server-side and never returned; verification tokens are never exposed.
type UserAdminService interface {
	RequireAuthenticated() error

	// ListUsers lists portal user accounts with optional filtering, sorting and
	// pagination. The returned int is the backend total (before pagination).
	ListUsers(ctx context.Context, params *admin.UserListParams) ([]*admin.User, int, error)

	// CreateUser creates a portal account with an email and password, plus
	// optional names and verification-email behavior.
	CreateUser(ctx context.Context, req *admin.UserCreateRequest) (*admin.User, error)

	// GetUser returns a single portal user account by numeric ID.
	GetUser(ctx context.Context, id int) (*admin.User, error)

	// UpdateUser patches an existing portal user. Omitted fields are left
	// unchanged.
	UpdateUser(ctx context.Context, id int, req *admin.UserUpdateRequest) (*admin.User, error)

	// DeleteUser removes a portal user account by ID.
	DeleteUser(ctx context.Context, id int) error
}

// RequireAuthenticated checks if the admin service is authenticated.
func (s *userAdminService) RequireAuthenticated() error {
	return s.base.RequireAuthenticated()
}

// getService returns the user service, lazily initializing with token exchange
// if needed. Initialization runs under the write lock with a re-check so
// concurrent cold-start callers do not perform redundant token
// exchanges/client creations.
func (s *userAdminService) getService(ctx context.Context) (*admin.UserService, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.service != nil {
		return s.service, nil
	}

	token, err := s.base.tokenProvider.GetLoginToken(ctx)
	if err != nil {
		return nil, err
	}

	client, err := admin.NewClient(
		admin.WithEndpoint(s.base.endpoint),
		admin.WithJWT(token),
	)
	if err != nil {
		return nil, err
	}

	s.service = client.Users()
	return s.service, nil
}

// ListUsers lists portal user accounts.
func (s *userAdminService) ListUsers(ctx context.Context, params *admin.UserListParams) ([]*admin.User, int, error) {
	return with3(s, ctx, func(svc *admin.UserService) ([]*admin.User, int, error) {
		return svc.ListUsers(ctx, params)
	})
}

// CreateUser creates a portal account.
func (s *userAdminService) CreateUser(ctx context.Context, req *admin.UserCreateRequest) (*admin.User, error) {
	return with2(s, ctx, func(svc *admin.UserService) (*admin.User, error) {
		return svc.CreateUser(ctx, req)
	})
}

// GetUser returns a portal user by ID.
func (s *userAdminService) GetUser(ctx context.Context, id int) (*admin.User, error) {
	return with2(s, ctx, func(svc *admin.UserService) (*admin.User, error) {
		return svc.GetUser(ctx, id)
	})
}

// UpdateUser patches a portal user.
func (s *userAdminService) UpdateUser(ctx context.Context, id int, req *admin.UserUpdateRequest) (*admin.User, error) {
	return with2(s, ctx, func(svc *admin.UserService) (*admin.User, error) {
		return svc.UpdateUser(ctx, id, req)
	})
}

// DeleteUser removes a portal user by ID.
func (s *userAdminService) DeleteUser(ctx context.Context, id int) error {
	return with0(s, ctx, func(svc *admin.UserService) error {
		return svc.DeleteUser(ctx, id)
	})
}
