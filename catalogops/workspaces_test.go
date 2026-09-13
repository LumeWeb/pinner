package catalogops

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ipfs "go.lumeweb.com/ipfs-sdk"
	"go.lumeweb.com/opmesh"
	"go.lumeweb.com/pinner/core/config"
	configmocks "go.lumeweb.com/pinner/core/config/mocks"
	"go.lumeweb.com/pinner/core/workspaces"
)

// workspacesService mocks workspaces.Service for the workspaces_* handlers,
// overriding only the methods each test exercises. It embeds the real
// interface so unimplemented methods panic rather than silently succeed.
type workspacesService struct {
	workspaces.Service

	authErr   error
	listFn    func(ctx context.Context, opts workspaces.ListOptions) ([]ipfs.WorkspaceResponse, error)
	getFn     func(ctx context.Context, id string) (*ipfs.WorkspaceResponse, error)
	createFn  func(ctx context.Context, req ipfs.WorkspaceRequest) (*ipfs.WorkspaceResponse, error)
	deleteFn  func(ctx context.Context, id string) (*ipfs.WorkspaceResponse, error)
	accessFn  func(ctx context.Context, id string, rotate bool) (*ipfs.WorkspaceAccessResponse, error)
	attachFn  func(ctx context.Context, id string, websiteID int) (*ipfs.WorkspaceResponse, error)
	resumeFn  func(ctx context.Context, id string) (*ipfs.WorkspaceResponse, error)
	suspendFn func(ctx context.Context, id string) (*ipfs.WorkspaceResponse, error)

	createReqCapture *ipfs.WorkspaceRequest
	attachCapID      string
	attachCapSite    int
	accessCapID      string
	accessCapRotate  bool
}

func (f *workspacesService) RequireAuthenticated() error { return f.authErr }

func (f *workspacesService) List(ctx context.Context, opts workspaces.ListOptions) ([]ipfs.WorkspaceResponse, error) {
	if f.listFn != nil {
		return f.listFn(ctx, opts)
	}
	return nil, nil
}

func (f *workspacesService) Get(ctx context.Context, id string) (*ipfs.WorkspaceResponse, error) {
	if f.getFn != nil {
		return f.getFn(ctx, id)
	}
	return nil, nil
}

func (f *workspacesService) Create(ctx context.Context, req ipfs.WorkspaceRequest) (*ipfs.WorkspaceResponse, error) {
	f.createReqCapture = &req
	if f.createFn != nil {
		return f.createFn(ctx, req)
	}
	return nil, nil
}

func (f *workspacesService) Delete(ctx context.Context, id string) (*ipfs.WorkspaceResponse, error) {
	if f.deleteFn != nil {
		return f.deleteFn(ctx, id)
	}
	return nil, nil
}

func (f *workspacesService) Access(ctx context.Context, id string, rotate bool) (*ipfs.WorkspaceAccessResponse, error) {
	f.accessCapID = id
	f.accessCapRotate = rotate
	if f.accessFn != nil {
		return f.accessFn(ctx, id, rotate)
	}
	return nil, nil
}

func (f *workspacesService) Attach(ctx context.Context, id string, websiteID int) (*ipfs.WorkspaceResponse, error) {
	f.attachCapID = id
	f.attachCapSite = websiteID
	if f.attachFn != nil {
		return f.attachFn(ctx, id, websiteID)
	}
	return nil, nil
}

func (f *workspacesService) Resume(ctx context.Context, id string) (*ipfs.WorkspaceResponse, error) {
	if f.resumeFn != nil {
		return f.resumeFn(ctx, id)
	}
	return nil, nil
}

func (f *workspacesService) Suspend(ctx context.Context, id string) (*ipfs.WorkspaceResponse, error) {
	if f.suspendFn != nil {
		return f.suspendFn(ctx, id)
	}
	return nil, nil
}

func workspacesDeps(t testing.TB, fake *workspacesService) WorkspacesDeps {
	return WorkspacesDeps{
		CfgMgr: func() config.Manager { return configmocks.NewMockManager(t) },
		NewAuthenticated: func(_ config.Manager, _ bool, _ string) (workspaces.Service, error) {
			return fake, nil
		},
		ServiceFactory: func(_ config.Manager, _ bool, _ ...workspaces.Option) workspaces.Service {
			return fake
		},
		GetAuthToken: func() string { return "" },
	}
}

func TestWorkspacesOperations_DeclaresAllEight(t *testing.T) {
	ops := WorkspacesOperations(workspacesDeps(t, &workspacesService{}))
	names := make([]string, len(ops))
	for i, op := range ops {
		names[i] = op.Name()
	}
	assert.Equal(t, []string{
		"workspaces_list",
		"workspaces_get",
		"workspaces_create",
		"workspaces_attach",
		"workspaces_suspend",
		"workspaces_resume",
		"workspaces_access",
		"workspaces_delete",
	}, names)
}

func TestWorkspacesOperations_NoResolveExposed(t *testing.T) {
	// Runtime workspace resolve is intentionally NOT a user operation.
	ops := WorkspacesOperations(workspacesDeps(t, &workspacesService{}))
	for _, op := range ops {
		assert.NotEqual(t, "workspaces_resolve", op.Name())
	}
}

func TestWorkspacesOperations_DeleteIsDestructiveAndHumanSafe(t *testing.T) {
	ops := WorkspacesOperations(workspacesDeps(t, &workspacesService{}))
	var del opmesh.Operation
	for _, op := range ops {
		if op.Name() == "workspaces_delete" {
			del = op
		}
	}
	require.NotNil(t, del)
	assert.Equal(t, opmesh.SafetyDestructive, del.Safety())
	// Model actors may run delete only with an explicit confirm (AgentConfirm);
	// the interaction surface itself remains AgentSafe (guarded by confirm).
	assert.Equal(t, opmesh.InteractionAgentSafe, del.Interaction())
}

func TestWorkspacesOperations_AccessIsHumanOnly(t *testing.T) {
	ops := WorkspacesOperations(workspacesDeps(t, &workspacesService{}))
	var acc opmesh.Operation
	for _, op := range ops {
		if op.Name() == "workspaces_access" {
			acc = op
		}
	}
	require.NotNil(t, acc)
	// Sensitive credentials: model actors are refused and handed off.
	assert.Equal(t, opmesh.InteractionHumanOnly, acc.Interaction())
	// Classified as a mutation: rotate=true replaces the proxy credential, so
	// the tool must not be advertised read-only to frontends.
	assert.Equal(t, opmesh.SafetyMutate, acc.Safety())
	// Still discoverable (search/describe surface it).
	assert.Equal(t, opmesh.VisibilityBoth, acc.Visibility())
}

func TestWorkspacesList_RequiresAuth(t *testing.T) {
	fake := &workspacesService{authErr: errors.New("not authenticated")}
	op := workspacesList(workspacesDeps(t, fake))
	_, err := op.Handler().Execute(context.Background(), map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not authenticated")
}

func TestWorkspacesList_ReturnsListResult(t *testing.T) {
	now := time.Now()
	fake := &workspacesService{
		listFn: func(_ context.Context, _ workspaces.ListOptions) ([]ipfs.WorkspaceResponse, error) {
			return []ipfs.WorkspaceResponse{{
				Id: 1, Domain: "ws1.example.com", Label: "ws1", Status: "running",
				Created: now, WebsiteId: intPtr(7),
			}}, nil
		},
	}
	op := workspacesList(workspacesDeps(t, fake))
	res, err := op.Handler().Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	lr, ok := res.(ListResult)
	require.True(t, ok)
	assert.Equal(t, 1, lr.ListCount())
	assert.Equal(t, []string{"ID", "DOMAIN", "LABEL", "STATUS", "WEBSITE ID", "CREATED"}, lr.ListHeaders())
	// attached workspace renders its website id; the created cell is the
	// formatted timestamp.
	require.Len(t, lr.ListRows(), 1)
	row := lr.ListRows()[0]
	assert.Equal(t, []string{"1", "ws1.example.com", "ws1", "running", "7"}, row[:5])
	assert.Equal(t, now.Format("2006-01-02 15:04:05"), row[5])
}

func TestWorkspacesList_UnattachedRendersDash(t *testing.T) {
	fake := &workspacesService{
		listFn: func(_ context.Context, _ workspaces.ListOptions) ([]ipfs.WorkspaceResponse, error) {
			return []ipfs.WorkspaceResponse{{Id: 2, Domain: "ws2.example.com", Label: "ws2", Status: "suspended"}}, nil
		},
	}
	op := workspacesList(workspacesDeps(t, fake))
	res, err := op.Handler().Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	lr := res.(ListResult)
	require.Len(t, lr.ListRows(), 1)
	assert.Equal(t, "-", lr.ListRows()[0][4])
}

func TestWorkspacesGet_RequiresID(t *testing.T) {
	fake := &workspacesService{}
	op := workspacesGet(workspacesDeps(t, fake))
	_, err := op.Handler().Execute(context.Background(), map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace id is required")
}

func TestWorkspacesGet_ReturnsWorkspace(t *testing.T) {
	fake := &workspacesService{
		getFn: func(_ context.Context, id string) (*ipfs.WorkspaceResponse, error) {
			assert.Equal(t, "42", id)
			return &ipfs.WorkspaceResponse{Id: 42, Domain: "ws.example.com", Status: "running"}, nil
		},
	}
	op := workspacesGet(workspacesDeps(t, fake))
	res, err := op.Handler().Execute(context.Background(), map[string]any{"id": 42})
	require.NoError(t, err)
	ws, ok := res.(*ipfs.WorkspaceResponse)
	require.True(t, ok)
	assert.Equal(t, 42, ws.Id)
}

func TestWorkspacesCreate_UnattachedDefault(t *testing.T) {
	fake := &workspacesService{
		createFn: func(_ context.Context, _ ipfs.WorkspaceRequest) (*ipfs.WorkspaceResponse, error) {
			return &ipfs.WorkspaceResponse{Id: 9, Domain: "ws9.example.com", Status: "running"}, nil
		},
	}
	op := workspacesCreate(workspacesDeps(t, fake))
	_, err := op.Handler().Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, fake.createReqCapture)
	assert.Nil(t, fake.createReqCapture.WebsiteId)
}

func TestWorkspacesCreate_WithWebsiteID(t *testing.T) {
	fake := &workspacesService{
		createFn: func(_ context.Context, _ ipfs.WorkspaceRequest) (*ipfs.WorkspaceResponse, error) {
			return &ipfs.WorkspaceResponse{Id: 9, Status: "running"}, nil
		},
	}
	op := workspacesCreate(workspacesDeps(t, fake))
	_, err := op.Handler().Execute(context.Background(), map[string]any{"website-id": 7})
	require.NoError(t, err)
	require.NotNil(t, fake.createReqCapture)
	require.NotNil(t, fake.createReqCapture.WebsiteId)
	assert.Equal(t, 7, *fake.createReqCapture.WebsiteId)
}

func TestWorkspacesCreate_InvalidWebsiteID(t *testing.T) {
	fake := &workspacesService{}
	op := workspacesCreate(workspacesDeps(t, fake))
	_, err := op.Handler().Execute(context.Background(), map[string]any{"website-id": "not-a-number"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid website-id")
}

func TestWorkspacesAttach_PassesIDs(t *testing.T) {
	fake := &workspacesService{
		attachFn: func(_ context.Context, _ string, _ int) (*ipfs.WorkspaceResponse, error) {
			return &ipfs.WorkspaceResponse{Id: 3, Domain: "ws.example.com", Status: "running", WebsiteId: intPtr(7)}, nil
		},
	}
	op := workspacesAttach(workspacesDeps(t, fake))
	res, err := op.Handler().Execute(context.Background(), map[string]any{"id": 3, "website-id": 7})
	require.NoError(t, err)
	assert.Equal(t, "3", fake.attachCapID)
	assert.Equal(t, 7, fake.attachCapSite)
	assert.NotNil(t, res)
}

func TestWorkspacesAttach_MissingWebsiteID(t *testing.T) {
	fake := &workspacesService{}
	op := workspacesAttach(workspacesDeps(t, fake))
	_, err := op.Handler().Execute(context.Background(), map[string]any{"id": 3})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "website-id is required")
}

func TestWorkspacesSuspendResume_CallThrough(t *testing.T) {
	fake := &workspacesService{
		suspendFn: func(_ context.Context, id string) (*ipfs.WorkspaceResponse, error) {
			assert.Equal(t, "5", id)
			return &ipfs.WorkspaceResponse{Id: 5, Status: "suspended"}, nil
		},
		resumeFn: func(_ context.Context, id string) (*ipfs.WorkspaceResponse, error) {
			assert.Equal(t, "5", id)
			return &ipfs.WorkspaceResponse{Id: 5, Status: "running"}, nil
		},
	}
	op := workspacesSuspend(workspacesDeps(t, fake))
	res, err := op.Handler().Execute(context.Background(), map[string]any{"id": 5})
	require.NoError(t, err)
	assert.Equal(t, "suspended", res.(*ipfs.WorkspaceResponse).Status)

	op = workspacesResume(workspacesDeps(t, fake))
	res, err = op.Handler().Execute(context.Background(), map[string]any{"id": 5})
	require.NoError(t, err)
	assert.Equal(t, "running", res.(*ipfs.WorkspaceResponse).Status)
}

func TestWorkspacesAccess_PassesRotate(t *testing.T) {
	fake := &workspacesService{
		accessFn: func(_ context.Context, _ string, _ bool) (*ipfs.WorkspaceAccessResponse, error) {
			return &ipfs.WorkspaceAccessResponse{Username: "u", Password: "p"}, nil
		},
	}
	op := workspacesAccess(workspacesDeps(t, fake))
	res, err := op.Handler().Execute(context.Background(), map[string]any{"id": 5, "rotate": true})
	require.NoError(t, err)
	assert.Equal(t, "5", fake.accessCapID)
	assert.True(t, fake.accessCapRotate)
	assert.Equal(t, "u", res.(*ipfs.WorkspaceAccessResponse).Username)
}

func TestWorkspacesAccess_DefaultsRotateFalse(t *testing.T) {
	fake := &workspacesService{
		accessFn: func(_ context.Context, _ string, _ bool) (*ipfs.WorkspaceAccessResponse, error) {
			return &ipfs.WorkspaceAccessResponse{}, nil
		},
	}
	op := workspacesAccess(workspacesDeps(t, fake))
	_, err := op.Handler().Execute(context.Background(), map[string]any{"id": 5})
	require.NoError(t, err)
	assert.False(t, fake.accessCapRotate)
}

func TestWorkspacesDelete_RequiresConfirm(t *testing.T) {
	fake := &workspacesService{}
	op := workspacesDelete(workspacesDeps(t, fake))
	_, err := op.Handler().Execute(context.Background(), map[string]any{"id": 5})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "confirmation is required")
}

func TestWorkspacesDelete_ReturnsResult(t *testing.T) {
	fake := &workspacesService{
		deleteFn: func(_ context.Context, _ string) (*ipfs.WorkspaceResponse, error) {
			return &ipfs.WorkspaceResponse{Id: 5, Status: "deleting"}, nil
		},
	}
	op := workspacesDelete(workspacesDeps(t, fake))
	res, err := op.Handler().Execute(context.Background(), map[string]any{"id": 5, "confirm": true})
	require.NoError(t, err)
	del, ok := res.(*WorkspaceDeleteResult)
	require.True(t, ok)
	assert.Equal(t, "5", del.ID)
	assert.Equal(t, "deleting", del.Status)
}
