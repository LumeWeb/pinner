package catalogops

import (
	"context"
	"fmt"

	"go.lumeweb.com/opmesh"
)

// adminWebsitesBlock is the `admin websites block` operation. Returns
// *admin.Website.
func adminWebsitesBlock(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_websites_block",
		Title:       "Block a website",
		Summary:     "Block a website by ID",
		Description: "Block an IPFS website by its ID. Requires admin privileges. Returns the updated website.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<website-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeString, Required: true, Help: "Website ID to block"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.websites()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.StrArg(input, "id", "")
			if id == "" {
				return nil, fmt.Errorf("admin_websites_block: website ID is required")
			}
			return svc.BlockWebsite(ctx, id)
		}),
	})
}

// adminWebsitesUnblock is the `admin websites unblock` operation. Returns
// *admin.Website.
func adminWebsitesUnblock(d AdminDeps) opmesh.Operation {
	return opmesh.NewOperation(opmesh.OperationSpec{
		Name:        "admin_websites_unblock",
		Title:       "Unblock a website",
		Summary:     "Unblock a website by ID",
		Description: "Unblock a previously blocked IPFS website by its ID. Requires admin privileges. Returns the updated website.",
		Category:    "admin",
		Safety:      opmesh.SafetyMutate,
		Interaction: opmesh.InteractionAgentSafe,
		Visibility:  opmesh.VisibilityBoth,
		Positional:  "<website-id>",
		Args: []opmesh.OperationArg{
			{Name: "id", Type: opmesh.ArgTypeString, Required: true, Help: "Website ID to unblock"},
		},
		Handler: handler(func(ctx context.Context, input map[string]any) (any, error) {
			svc, err := d.websites()
			if err != nil {
				return nil, err
			}
			if err := svc.RequireAuthenticated(); err != nil {
				return nil, err
			}
			id := opmesh.StrArg(input, "id", "")
			if id == "" {
				return nil, fmt.Errorf("admin_websites_unblock: website ID is required")
			}
			return svc.UnblockWebsite(ctx, id)
		}),
	})
}
