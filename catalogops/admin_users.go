package catalogops

import (
	"context"
	"fmt"

	"go.lumeweb.com/opmesh"
	"go.lumeweb.com/portal-sdk/admin"
)

// Operation names for the admin users section (kept for the CLI wiring layer
// and tests).
const (
	OpAdminUsersList   = "admin_users_list"
	OpAdminUsersGet    = "admin_users_get"
	OpAdminUsersCreate = "admin_users_create"
	OpAdminUsersUpdate = "admin_users_update"
	OpAdminUsersDelete = "admin_users_delete"
	OpAdminUsersVerify = "admin_users_verify"
)

// adminUsersListResult builds the shared ListResult view for the users list
// operation.
func adminUsersListResult(users []*admin.User, total int) ListResult {
	headers := []string{"ID", "EMAIL", "NAME", "ROLE", "VERIFIED"}
	rows := make([][]string, 0, len(users))
	for _, u := range users {
		name := u.FirstName
		if name != "" && u.LastName != "" {
			name += " " + u.LastName
		} else if name == "" {
			name = u.LastName
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", u.Id), u.Email, name, u.Role, adminYesNo(u.Verified),
		})
	}
	return NewListResult(users, ListResultMeta{Noun: "user(s)", Headers: headers, Rows: rows, Total: total})
}

// UsersDeleteResult reports a deleted admin user.
type UsersDeleteResult struct {
	Deleted bool `json:"deleted"`
	ID      int  `json:"id"`
}

// adminUsersList is the `admin users list` operation.
func adminUsersList(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        OpAdminUsersList,
		Title:       "List users",
		Summary:     "List portal user accounts",
		Description: "List portal user accounts with optional email/verified filtering and pagination. Passwords are never returned. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Args: append(opmesh.ListArgs(),
			opmesh.OperationArg{Name: "email", Type: opmesh.ArgTypeString, Help: "Filter by exact email address"},
			opmesh.OperationArg{Name: "verified", Type: opmesh.ArgTypeNullableBool, Help: "Filter by verification state (true/false)"},
		),
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.users()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			params := &admin.UserListParams{}
			if v := opmesh.StrArg(input, "email", ""); v != "" {
				params.Email = &v
			}
			if v := opmesh.BoolArgPtr(input, "verified"); v != nil {
				params.Verified = v
			}
			// Server-side paging: map the page/page-size cursor onto the
			// admin API's queryutil `_start`/`_end` window instead of
			// fetching the whole table and slicing client-side. Fetching
			// then slicing made every page return the first N rows (the
			// backend always defaulted to _start=0,_end=10). Passing the
			// window on to ListUsers lets the backend return the correct
			// slice directly.
			page := opmesh.ParseListPage(input, 10)
			start := page.Start
			end := page.Start + page.Limit
			params.UnderscoreStart = &start
			params.UnderscoreEnd = &end
			users, total, err := svc.ListUsers(ctx, params)
			if err != nil {
				return nil, err
			}
			return adminUsersListResult(users, total), nil
		}),
	})
}

// adminUsersGet is the `admin users get` operation.
func adminUsersGet(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        OpAdminUsersGet,
		Title:       "Get a user",
		Summary:     "Get a portal user by ID",
		Description: "Get a single portal user account by numeric ID. Passwords are never returned. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyRead,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<user-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeInt, Required: true, Help: "User ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.users()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.IntArg(input, "id", 0)
			if id <= 0 {
				return nil, fmt.Errorf("admin_users_get: user ID is required")
			}
			return svc.GetUser(ctx, id)
		}),
	})
}

// adminUsersVerify is the `admin users verify` operation. It is the
// single-purpose form of `admin_users_update` for flipping the
// email-verification state without forcing every caller to spell out the
// patch-protocol of the full update surface.
func adminUsersVerify(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        OpAdminUsersVerify,
		Title:       "Verify a user",
		Summary:     "Mark a user's email as verified",
		Description: "Mark a portal user's email as verified without sending a verification email. To unverify, or to change other fields, use the update operation. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<user-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeInt, Required: true, Help: "User ID"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.users()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.IntArg(input, "id", 0)
			if id <= 0 {
				return nil, fmt.Errorf("admin_users_verify: user ID is required")
			}
			verified := true
			return svc.UpdateUser(ctx, id, &admin.UserUpdateRequest{Verified: &verified})
		}),
	})
}

// adminUsersCreate is the `admin users create` operation.
func adminUsersCreate(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        OpAdminUsersCreate,
		Title:       "Create a user",
		Summary:     "Create a portal user account",
		Description: "Create a new portal user account with an email and password, plus optional first/last name and verification-email behavior. The password is hashed server-side and never returned. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Args: []opmesh.OperationArg{
			{Name: "email", Type: opmesh.ArgTypeString, Required: true, Help: "User email address"},
			{Name: "password", Type: opmesh.ArgTypeString, Required: true, Help: "Initial password (hashed server-side)"},
			{Name: "first-name", Type: opmesh.ArgTypeString, Help: "First name"},
			{Name: "last-name", Type: opmesh.ArgTypeString, Help: "Last name"},
			{Name: "verify-email", Type: opmesh.ArgTypeBool, Help: "Send a verification email"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.users()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			req := &admin.UserCreateRequest{
				Email:       opmesh.StrArg(input, "email", ""),
				Password:    opmesh.StrArg(input, "password", ""),
				VerifyEmail: opmesh.BoolArg(input, "verify-email", false),
			}
			if v := opmesh.StrArg(input, "first-name", ""); v != "" {
				req.FirstName = &v
			}
			if v := opmesh.StrArg(input, "last-name", ""); v != "" {
				req.LastName = &v
			}
			if req.Email == "" {
				return nil, fmt.Errorf("admin_users_create: email is required")
			}
			if req.Password == "" {
				return nil, fmt.Errorf("admin_users_create: password is required")
			}
			return svc.CreateUser(ctx, req)
		}),
	})
}

// adminUsersUpdate is the `admin users update` operation.
func adminUsersUpdate(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:    OpAdminUsersUpdate,
		Title:   "Update a user",
		Summary: "Update a portal user account",
		// Nullable arg types matter for updates: omitted must be distinguishable
		// from the zero value, else an update without the verified flag would
		// arrive as verified=false and the backend would unverify the user.
		Description: "Update an existing portal user account. Only the fields provided are changed; others keep their current values. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<user-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeInt, Required: true, Help: "User ID"},
			{Name: "email", Type: opmesh.ArgTypeString, Help: "User email address"},
			{Name: "first-name", Type: opmesh.ArgTypeString, Help: "First name"},
			{Name: "last-name", Type: opmesh.ArgTypeString, Help: "Last name"},
			{Name: "password", Type: opmesh.ArgTypeString, Help: "New password (hashed server-side)"},
			{Name: "verified", Type: opmesh.ArgTypeNullableBool, Help: "Set the email-verification state"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.users()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.IntArg(input, "id", 0)
			if id <= 0 {
				return nil, fmt.Errorf("admin_users_update: user ID is required")
			}
			// Nil fields are sent omitted and the backend leaves them unchanged.
			req := &admin.UserUpdateRequest{}
			if v := opmesh.StrArg(input, "email", ""); v != "" {
				req.Email = &v
			}
			if v := opmesh.StrArg(input, "first-name", ""); v != "" {
				req.FirstName = &v
			}
			if v := opmesh.StrArg(input, "last-name", ""); v != "" {
				req.LastName = &v
			}
			if v := opmesh.StrArg(input, "password", ""); v != "" {
				req.Password = &v
			}
			if v := opmesh.BoolArgPtr(input, "verified"); v != nil {
				req.Verified = v
			}
			return svc.UpdateUser(ctx, id, req)
		}),
	})
}

// adminUsersDelete is the `admin users delete` operation.
// DESTRUCTIVE: requires confirm=true.
func adminUsersDelete(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        OpAdminUsersDelete,
		Title:       "Delete a user",
		Summary:     "Delete a portal user account by ID",
		Description: "Delete a portal user account by ID. DESTRUCTIVE: the account and its data are removed. Requires confirm=true. Requires admin privileges.",
		Category:    "admin",
		Safety:      opmesh.SafetyDestructive,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<user-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeInt, Required: true, Help: "User ID"},
			{Name: "confirm", Type: opmesh.ArgTypeBool, Required: true, Help: "Confirm the destructive delete"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			if !opmesh.BoolArg(input, "confirm", false) {
				return nil, fmt.Errorf("admin_users_delete: confirmation is required")
			}
			svc, err := d.users()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.IntArg(input, "id", 0)
			if id <= 0 {
				return nil, fmt.Errorf("admin_users_delete: user ID is required")
			}
			if err := svc.DeleteUser(ctx, id); err != nil {
				return nil, err
			}
			return &UsersDeleteResult{Deleted: true, ID: id}, nil
		}),
	})
}
