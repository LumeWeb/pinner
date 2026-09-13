package catalogops

import (
	"context"
	"reflect"
	"strings"
	"testing"

	coreadmin "go.lumeweb.com/pinner/core/admin"
	"go.lumeweb.com/pinner/core/config"
	configmocks "go.lumeweb.com/pinner/core/config/mocks"
	"go.lumeweb.com/portal-sdk/admin"
)

// fakeUserService is a hand-rolled admin.UserAdminService whose methods are
// driven by function fields, so tests can assert both the plumbing
// (RequireAuthenticated gating, argument forwarding) and the op result
// wrapping without mocks.
type fakeUserService struct {
	requireAuth func() error
	listFn      func(ctx context.Context, params *admin.UserListParams) ([]*admin.User, int, error)
	getFn       func(ctx context.Context, id int) (*admin.User, error)
	createFn    func(ctx context.Context, req *admin.UserCreateRequest) (*admin.User, error)
	updateFn    func(ctx context.Context, id int, req *admin.UserUpdateRequest) (*admin.User, error)
	deleteFn    func(ctx context.Context, id int) error
}

func (f *fakeUserService) RequireAuthenticated() error {
	if f.requireAuth != nil {
		return f.requireAuth()
	}
	return nil
}

func (f *fakeUserService) ListUsers(ctx context.Context, params *admin.UserListParams) ([]*admin.User, int, error) {
	if f.listFn != nil {
		return f.listFn(ctx, params)
	}
	return nil, 0, nil
}

func (f *fakeUserService) GetUser(ctx context.Context, id int) (*admin.User, error) {
	if f.getFn != nil {
		return f.getFn(ctx, id)
	}
	return nil, nil
}

func (f *fakeUserService) CreateUser(ctx context.Context, req *admin.UserCreateRequest) (*admin.User, error) {
	if f.createFn != nil {
		return f.createFn(ctx, req)
	}
	return nil, nil
}

func (f *fakeUserService) UpdateUser(ctx context.Context, id int, req *admin.UserUpdateRequest) (*admin.User, error) {
	if f.updateFn != nil {
		return f.updateFn(ctx, id, req)
	}
	return nil, nil
}

func (f *fakeUserService) DeleteUser(ctx context.Context, id int) error {
	if f.deleteFn != nil {
		return f.deleteFn(ctx, id)
	}
	return nil
}

// testUsersDeps wires a fake user service into an AdminDeps whose CfgMgr
// returns a fresh config mock.
func testUsersDeps(t *testing.T, svc *fakeUserService) AdminDeps {
	return AdminDeps{
		CfgMgr: func() config.Manager { return configmocks.NewMockManager(t) },
		UserAdminService: func(cfgMgr config.Manager) (coreadmin.UserAdminService, error) {
			return svc, nil
		},
	}
}

// sampleUser returns a user with a few fields filled in. admin.User embeds an
// unexported generated response type, so its promoted fields are populated via
// reflection (direct struct literals cannot reference the internal package).
func sampleUser() *admin.User {
	u := &admin.User{}
	set := []struct {
		name string
		val  any
	}{
		{"Id", int64(3)},
		{"Email", "admin@example.com"},
		{"FirstName", "Ada"},
		{"LastName", "Lovelace"},
		{"Role", "user"},
		{"Verified", true},
	}
	v := reflect.ValueOf(u).Elem()
	for _, s := range set {
		f := v.FieldByName(s.name)
		if !f.IsValid() || !f.CanSet() {
			panic("sampleUser: field " + s.name + " not settable")
		}
		switch f.Kind() {
		case reflect.Int, reflect.Int64:
			f.SetInt(s.val.(int64))
		case reflect.String:
			f.SetString(s.val.(string))
		case reflect.Bool:
			f.SetBool(s.val.(bool))
		}
	}
	return u
}

// TestAdminOperationsReturnsUsers asserts the registry includes the admin user
// CRUD operations.
func TestAdminOperationsReturnsUsers(t *testing.T) {
	ops := AdminOperations(AdminDeps{})
	names := map[string]bool{}
	for _, op := range ops {
		names[op.Name()] = true
	}
	for _, want := range []string{
		"admin_users_list",
		"admin_users_get",
		"admin_users_create",
		"admin_users_update",
		"admin_users_delete",
	} {
		if !names[want] {
			t.Fatalf("AdminOperations missing expected op %q", want)
		}
	}
}

// TestAdminUsersListNilDeps asserts an unwired service getter degrades to a
// clear error rather than panicking.
func TestAdminUsersListNilDeps(t *testing.T) {
	op := adminUsersList(AdminDeps{})
	_, err := op.Handler().Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected an error when the user service is not wired")
	}
}

// TestAdminUsersList asserts gating, filter forwarding and result wrapping.
func TestAdminUsersList(t *testing.T) {
	var gotParams *admin.UserListParams
	svc := &fakeUserService{
		requireAuth: func() error { return nil },
		listFn: func(ctx context.Context, params *admin.UserListParams) ([]*admin.User, int, error) {
			gotParams = params
			return []*admin.User{sampleUser()}, 1, nil
		},
	}
	op := adminUsersList(testUsersDeps(t, svc))
	res, err := op.Handler().Execute(context.Background(), map[string]any{
		"email":    "admin@example.com",
		"verified": true,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if gotParams == nil {
		t.Fatal("list params did not reach the service")
	}
	if gotParams.Email == nil || *gotParams.Email != "admin@example.com" {
		t.Fatalf("email filter not forwarded: %+v", gotParams)
	}
	if gotParams.Verified == nil || !*gotParams.Verified {
		t.Fatalf("verified filter not forwarded: %+v", gotParams)
	}
	got, ok := res.(ListResult)
	if !ok {
		t.Fatalf("unexpected result type %T", res)
	}
	if got.ListCount() != 1 {
		t.Fatalf("unexpected count: %d", got.ListCount())
	}
	if _, ok := got.ListItems().([]*admin.User); !ok {
		t.Fatalf("unexpected result: %+v", got.ListItems())
	}
}

// TestAdminUsersListPagingArgs asserts the op embeds the shared pager args and
// that page/page-size slice the fetched result set client-side.
func TestAdminUsersListPagingArgs(t *testing.T) {
	op := adminUsersList(AdminDeps{})
	argNames := map[string]bool{}
	for _, arg := range op.Args() {
		argNames[arg.Name] = true
	}
	for _, want := range []string{"page", "page-size", "email", "verified"} {
		if !argNames[want] {
			t.Fatalf("admin_users_list missing arg %q", want)
		}
	}

	svc := &fakeUserService{
		requireAuth: func() error { return nil },
		listFn: func(ctx context.Context, params *admin.UserListParams) ([]*admin.User, int, error) {
			users := make([]*admin.User, 0, 3)
			for i := 0; i < 3; i++ {
				u := sampleUser()
				u.Id = i + 1
				users = append(users, u)
			}
			return users, 3, nil
		},
	}
	op = adminUsersList(testUsersDeps(t, svc))
	res, err := op.Handler().Execute(context.Background(), map[string]any{
		"page":      2,
		"page-size": 1,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got, ok := res.(ListResult)
	if !ok {
		t.Fatalf("unexpected result type %T", res)
	}
	if got.ListCount() != 1 {
		t.Fatalf("page=2 page-size=1 should slice to 1 row, got %d", got.ListCount())
	}
	items, ok := got.ListItems().([]*admin.User)
	if !ok || len(items) != 1 || items[0].Id != 2 {
		t.Fatalf("expected the second user on page 2, got %+v", got.ListItems())
	}
}

// TestAdminUsersListAuthGate asserts RequireAuthenticated is honored.
func TestAdminUsersListAuthGate(t *testing.T) {
	svc := &fakeUserService{requireAuth: func() error { return context.Canceled }}
	op := adminUsersList(testUsersDeps(t, svc))
	_, err := op.Handler().Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected auth gate to reject the call")
	}
}

// TestAdminUsersGetForwarding verifies the numeric id is forwarded as an int.
func TestAdminUsersGetForwarding(t *testing.T) {
	var gotID int
	svc := &fakeUserService{
		requireAuth: func() error { return nil },
		getFn: func(ctx context.Context, id int) (*admin.User, error) {
			gotID = id
			return sampleUser(), nil
		},
	}
	op := adminUsersGet(testUsersDeps(t, svc))
	_, err := op.Handler().Execute(context.Background(), map[string]any{"id": 3})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if gotID != 3 {
		t.Fatalf("unexpected id %d", gotID)
	}
}

// TestAdminUsersCreateForwarding verifies required-field validation and that
// the request reaches the service untouched.
func TestAdminUsersCreateForwarding(t *testing.T) {
	var gotReq *admin.UserCreateRequest
	svc := &fakeUserService{
		requireAuth: func() error { return nil },
		createFn: func(ctx context.Context, req *admin.UserCreateRequest) (*admin.User, error) {
			gotReq = req
			return sampleUser(), nil
		},
	}
	op := adminUsersCreate(testUsersDeps(t, svc))
	_, err := op.Handler().Execute(context.Background(), map[string]any{
		"email":        "new@example.com",
		"password":     "hunter2",
		"first-name":   "Grace",
		"last-name":    "Hopper",
		"verify-email": true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if gotReq == nil {
		t.Fatal("create request did not reach the service")
	}
	if gotReq.Email != "new@example.com" || gotReq.Password != "hunter2" || !gotReq.VerifyEmail {
		t.Fatalf("unexpected request: %+v", gotReq)
	}
	if gotReq.FirstName == nil || *gotReq.FirstName != "Grace" {
		t.Fatalf("first name not applied: %+v", gotReq)
	}
	if gotReq.LastName == nil || *gotReq.LastName != "Hopper" {
		t.Fatalf("last name not applied: %+v", gotReq)
	}
}

// TestAdminUsersCreateValidation asserts required-field errors fire before the
// service is called.
func TestAdminUsersCreateValidation(t *testing.T) {
	cases := []struct{ key, missing string }{
		{"email", "email"},
		{"password", "password"},
	}
	for _, tc := range cases {
		op := adminUsersCreate(testUsersDeps(t, &fakeUserService{}))
		input := map[string]any{"email": "a@b.co", "password": "pw"}
		delete(input, tc.key)
		_, err := op.Handler().Execute(context.Background(), input)
		if err == nil || !strings.Contains(err.Error(), tc.missing) {
			t.Fatalf("expected error mentioning %q, got %v", tc.missing, err)
		}
	}
}

// TestAdminUsersUpdatePatchOnlySuppliedFields asserts supplied args map to the
// patch request's pointer fields while everything else stays nil.
func TestAdminUsersUpdatePatchOnlySuppliedFields(t *testing.T) {
	var gotReq *admin.UserUpdateRequest
	var gotID int
	svc := &fakeUserService{
		requireAuth: func() error { return nil },
		updateFn: func(ctx context.Context, id int, req *admin.UserUpdateRequest) (*admin.User, error) {
			gotID, gotReq = id, req
			return sampleUser(), nil
		},
	}
	op := adminUsersUpdate(testUsersDeps(t, svc))
	_, err := op.Handler().Execute(context.Background(), map[string]any{
		"id":        3,
		"last-name": "Hopper",
		"verified":  false,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if gotID != 3 {
		t.Fatalf("unexpected id %d", gotID)
	}
	if gotReq == nil {
		t.Fatal("update request did not reach the service")
	}
	if gotReq.LastName == nil || *gotReq.LastName != "Hopper" {
		t.Fatalf("override not applied: %+v", gotReq)
	}
	// Explicit false for verified is forwarded.
	if gotReq.Verified == nil || *gotReq.Verified != false {
		t.Fatalf("explicit verified=false not applied: %+v", gotReq)
	}
	// Omitted fields stay nil on the wire.
	if gotReq.Email != nil || gotReq.FirstName != nil || gotReq.Password != nil {
		t.Fatalf("omitted fields must not be set: %+v", gotReq)
	}
}

// TestAdminUsersDeleteRequiresConfirm asserts delete requires confirm=true
// before touching the service.
func TestAdminUsersDeleteRequiresConfirm(t *testing.T) {
	called := false
	svc := &fakeUserService{
		requireAuth: func() error { return nil },
		deleteFn: func(ctx context.Context, id int) error {
			called = true
			return nil
		},
	}
	op := adminUsersDelete(testUsersDeps(t, svc))
	_, err := op.Handler().Execute(context.Background(), map[string]any{"id": 3, "confirm": false})
	if err == nil || !strings.Contains(err.Error(), "confirmation is required") {
		t.Fatalf("expected confirmation error, got %v", err)
	}
	if called {
		t.Fatal("delete must not reach the service without confirmation")
	}
}

// TestAdminUsersDeleteConfirmed verifies a confirmed delete forwards the id
// and returns a UsersDeleteResult.
func TestAdminUsersDeleteConfirmed(t *testing.T) {
	var gotID int
	svc := &fakeUserService{
		requireAuth: func() error { return nil },
		deleteFn: func(ctx context.Context, id int) error {
			gotID = id
			return nil
		},
	}
	op := adminUsersDelete(testUsersDeps(t, svc))
	res, err := op.Handler().Execute(context.Background(), map[string]any{"id": 3, "confirm": true})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if gotID != 3 {
		t.Fatalf("unexpected id %d", gotID)
	}
	dr, ok := res.(*UsersDeleteResult)
	if !ok {
		t.Fatalf("unexpected result type %T", res)
	}
	if !dr.Deleted || dr.ID != 3 {
		t.Fatalf("unexpected delete result: %+v", dr)
	}
}
