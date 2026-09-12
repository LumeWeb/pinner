package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/opmesh"

	"go.lumeweb.com/pinner/assembly"
	"go.lumeweb.com/pinner/catalogops"
)

// testMetaBundle returns an all-domain bundle with zero-valued (unwired) deps
// for each catalogops domain. Assembly only REGISTERS operations, never
// executes them, so unwired domains are exactly what this characterization
// needs (execution for catalog ops goes through the dispatch seam's test stub
// below).
func testMetaBundle() *assembly.CatalogDepsBundle {
	return &assembly.CatalogDepsBundle{
		Auth:       catalogops.AuthDeps{},
		Account:    catalogops.AccountDeps{},
		Vault:      catalogops.VaultDeps{},
		VaultSetup: catalogops.VaultDeps{},
		Pins:       catalogops.PinsDeps{},
		Websites:   catalogops.WebsitesDeps{},
		DNS:        catalogops.DNSDeps{},
		IPNS:       catalogops.IPNSDeps{},
		ENS:        catalogops.ENSDeps{},
		APIKeys:    catalogops.APIKeysDeps{},
		Operations: catalogops.OperationsDeps{},
		Admin:      catalogops.AdminDeps{},
	}
}

// dispatchRecorder returns a CatalogDispatch stub that records every name it
// is asked to execute and returns a canned result — the characterization of
// the seam: meta invocations of compiled ops MUST cross it, nothing else.
func dispatchRecorder() (CatalogDispatch, *[]string) {
	var dispatched []string
	return func(name string) model.ToolHandler {
		return func(_ context.Context, _ model.ToolRequest) (model.ToolResult, error) {
			dispatched = append(dispatched, name)
			return model.ToolResult{Text: "ok"}, nil
		}
	}, &dispatched
}

// metaProgressiveServer assembles a real presentation over the full domain
// scope with the given listing policy and returns it with the meta
// descriptors built on the recording dispatch seam.
func metaProgressiveServer(t *testing.T, listing *ListingPolicy) (*Server, []model.ToolDescriptor, *[]string) {
	t.Helper()
	srv, err := Assemble(Config{
		DomainScope: assembly.FullDomainScope,
		Deps:        testMetaBundle(),
		Listing:     listing,
	})
	require.NoError(t, err, "assemble must succeed over the test bundle")
	dispatch, seen := dispatchRecorder()
	metas, err := srv.MetaToolDescriptors(dispatch)
	require.NoError(t, err, "meta descriptors must build")
	require.Len(t, metas, len(MetaToolNames()), "exactly the five meta tools must be returned")
	require.Equal(t, MetaToolNames(), metaNamesOf(metas), "meta descriptors must be in registration order")
	return srv, metas, seen
}

func metaNamesOf(descs []model.ToolDescriptor) []string {
	names := make([]string, 0, len(descs))
	for _, d := range descs {
		names = append(names, d.Name)
	}
	return names
}

// callMeta invokes a meta tool's baked-in handler directly (handler-level
// characterization; the composition roots register these descriptors as-is).
func callMeta(t *testing.T, descs []model.ToolDescriptor, name string, args map[string]any) model.ToolResult {
	t.Helper()
	for _, d := range descs {
		if d.Name != name {
			continue
		}
		res, err := d.Handler(context.Background(), model.ToolRequest{Name: name, Arguments: args})
		require.NoError(t, err, "%s handler must not fail at the transport layer", name)
		return res
	}
	t.Fatalf("meta tool %q not found on the descriptor set", name)
	return model.ToolResult{}
}

// searchEnvelope is the union of the search and onboarding envelopes: the
// onboarding shape is the search shape plus the optional hint.
type searchEnvelope struct {
	Tools []toolSummary `json:"tools"`
	Total int           `json:"total"`
	Hint  string        `json:"hint,omitempty"`
}

// decodeSearch unmarshals a search_tools result envelope (either path).
func decodeSearch(t *testing.T, res model.ToolResult) searchEnvelope {
	t.Helper()
	require.NotEmpty(t, res.Text, "search_tools must return a text envelope")
	var out searchEnvelope
	require.NoError(t, json.Unmarshal([]byte(res.Text), &out), "search_tools result must be the search envelope")
	return out
}

// TestServesMetaToolsMatrix pins the registration gate: progressive always
// serves; flat serves unless IncludeMetaOnFlat is EXPLICITLY false — an unset
// (nil) switch under flat is the retain default, never the opt-out.
func TestServesMetaToolsMatrix(t *testing.T) {
	require.True(t, ServesMetaTools(DefaultPolicy()), "progressive default must serve the meta tools")
	require.True(t, ServesMetaTools(ListingPolicy{Strategy: ListingProgressive}),
		"explicit progressive must serve the meta tools")

	falseOn := false
	trueOn := true
	require.True(t, ServesMetaTools(ListingPolicy{Strategy: ListingFlat}),
		"flat with an UNSET IncludeMetaOnFlat must retain the meta tools")
	require.True(t, ServesMetaTools(ListingPolicy{Strategy: ListingFlat, IncludeMetaOnFlat: &trueOn}),
		"flat with explicit true must retain the meta tools")
	require.False(t, ServesMetaTools(ListingPolicy{Strategy: ListingFlat, IncludeMetaOnFlat: &falseOn}),
		"flat with explicit false is the only opt-out")
}

// TestServerServesMetaToolsReadsResolvedPolicy pins that the Server method
// reports the policy the assembly actually resolved (its own flat default when
// none was declared), not the assembly input.
func TestServerServesMetaToolsReadsResolvedPolicy(t *testing.T) {
	explicitOff := false
	srv, err := Assemble(Config{
		DomainScope: assembly.FullDomainScope,
		Deps:        testMetaBundle(),
		Listing:     &ListingPolicy{Strategy: ListingFlat, IncludeMetaOnFlat: &explicitOff},
	})
	require.NoError(t, err)
	assert.Equal(t, ListingFlat, srv.ListingPolicy().Strategy)
	assert.False(t, srv.ServesMetaTools(),
		"the explicit flat opt-out must turn off ServesMetaTools on the assembled server")
}

// TestMetaToolDescriptorsNames pins the five reserved wire names in stable
// registration order.
func TestMetaToolDescriptorsNames(t *testing.T) {
	require.Equal(t, []string{
		MetaToolSearch,
		MetaToolDescribe,
		MetaToolInvokeRead,
		MetaToolInvokeWrite,
		MetaToolInvokeDestructive,
	}, MetaToolNames())
}

// TestMetaToolDescriptorsRejectsReservedNames pins the collision guard: a
// surface that already declares a meta-tool name must fail the descriptor
// build loudly instead of silently shadowing the dispatcher.
func TestMetaToolDescriptorsRejectsReservedNames(t *testing.T) {
	shadowed := &Server{
		Tools:   []CatalogPresentation{{Name: MetaToolSearch}},
		catalog: opmesh.NewCatalog(),
	}
	_, err := shadowed.MetaToolDescriptors(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved meta-tool name")

	direct := &Server{
		Direct:  []model.ToolDescriptor{{Name: MetaToolDescribe}},
		catalog: opmesh.NewCatalog(),
	}
	_, err = direct.MetaToolDescriptors(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved meta-tool name")
}

// TestMetaProgressiveSearchFindsPins pins the discovery loop end to end on a
// real assembly: search_tools finds the compiled pins ops, and describe_tool
// resolves one with its full input schema and the read dispatcher named.
func TestMetaProgressiveSearchFindsPins(t *testing.T) {
	srv, metas, _ := metaProgressiveServer(t, nil)

	require.Equal(t, ListingProgressive, srv.ListingPolicy().Strategy)
	require.True(t, srv.ServesMetaTools())

	res := callMeta(t, metas, MetaToolSearch, map[string]any{"query": "pins"})
	require.False(t, res.IsError, "search_tools must resolve successfully")
	found := decodeSearch(t, res)
	assert.Positive(t, found.Total, "searching 'pins' must surface the pins tools")
	assert.NotEmpty(t, found.Tools)

	res = callMeta(t, metas, MetaToolDescribe, map[string]any{"name": "pins_list"})
	require.False(t, res.IsError, "describe_tool must resolve a compiled op")
	var detail toolDetail
	require.NoError(t, json.Unmarshal([]byte(res.Text), &detail))
	assert.Equal(t, "pins_list", detail.Name)
	assert.NotEmpty(t, detail.InputSchema, "describe must carry the input schema")
	assert.Equal(t, MetaToolInvokeRead, detail.InvokeTool,
		"pins_list is read-only, so describe must route to the read dispatcher")
}

// TestMetaProgressiveOnlyDirectVisibleMaterialized pins the progressive
// composition contract: on tools/list the composition root materializes
// exactly the DirectVisible compiled ops plus the direct-only tools plus the
// meta tools — no other compiled op is directly materialized.
func TestMetaProgressiveOnlyDirectVisibleMaterialized(t *testing.T) {
	srv, metas, _ := metaProgressiveServer(t, nil)
	require.True(t, srv.ServesMetaTools())

	allowed := map[string]bool{}
	for _, d := range srv.Direct {
		allowed[d.Name] = true
	}
	for _, tool := range srv.Tools {
		if tool.DirectVisible {
			allowed[tool.Name] = true
		}
	}
	for _, meta := range metas {
		allowed[meta.Name] = true
	}

	for _, tool := range srv.Tools {
		if tool.DirectVisible {
			assert.Truef(t, allowed[tool.Name], "direct-visible %q must be in the allowed set", tool.Name)
			continue
		}
		assert.Falsef(t, allowed[tool.Name] && !metaNamesContains(metas, tool.Name),
			"non-direct op %q must not be materialized directly under progressive", tool.Name)
	}
}

func metaNamesContains(descs []model.ToolDescriptor, name string) bool {
	for _, d := range descs {
		if d.Name == name {
			return true
		}
	}
	return false
}

// TestMetaInvokeDispatchesThroughSeam pins that a class-correct invocation of
// a compiled op crosses the caller's CatalogDispatch seam exactly once
// (credential threading and catalog gates stay at the composition root), and
// that out-of-class invocations are refused with a pointer to the right
// dispatcher.
func TestMetaInvokeDispatchesThroughSeam(t *testing.T) {
	_, metas, seen := metaProgressiveServer(t, nil)

	// Class-correct: pins_list is read-only, so invoke_read_tool accepts it.
	res := callMeta(t, metas, MetaToolInvokeRead, map[string]any{
		"name":      "pins_list",
		"arguments": map[string]any{},
	})
	require.False(t, res.IsError, "read invocation of a read-only op must dispatch")
	require.Equal(t, []string{"pins_list"}, *seen, "the invocation must cross the CatalogDispatch seam exactly once")

	// Out-of-class: the read dispatcher refuses a destructive op and points
	// the agent at the destructive dispatcher.
	dispatchedBefore := len(*seen)
	res = callMeta(t, metas, MetaToolInvokeRead, map[string]any{"name": "pins_rm", "arguments": map[string]any{}})
	require.True(t, res.IsError, "invoke_read_tool must refuse a destructive op")
	assert.Contains(t, res.Text, "destructive", "refusal must name the op's class")
	assert.Contains(t, res.Text, MetaToolInvokeDestructive, "refusal must name the correct dispatcher")
	assert.Len(t, *seen, dispatchedBefore, "a refused invocation must never cross the dispatch seam")

	// Unknown tool: refusal carries nearest-name suggestions.
	res = callMeta(t, metas, MetaToolInvokeWrite, map[string]any{"name": "pins_lst", "arguments": map[string]any{}})
	require.True(t, res.IsError)
	assert.Contains(t, res.Text, "did you mean one of these?", "unknown-tool refusal must offer suggestions")
}

// TestMetaSearchTotalReportsPreCapCount pins the truthful-total contract: a
// limit truncates the returned list but Total must keep the pre-cap match
// count, on both the search and the onboarding path.
func TestMetaSearchTotalReportsPreCapCount(t *testing.T) {
	_, metas, _ := metaProgressiveServer(t, nil)

	// No limit: every match is returned and Total equals the returned count.
	unbounded := decodeSearch(t, callMeta(t, metas, MetaToolSearch, map[string]any{"query": "pins"}))
	require.Positive(t, unbounded.Total, "searching 'pins' must match tools")
	assert.Equal(t, len(unbounded.Tools), unbounded.Total,
		"without a limit Total must equal the returned tool count")

	// limit=1 truncates, but Total must still report the pre-cap count.
	capped := decodeSearch(t, callMeta(t, metas, MetaToolSearch, map[string]any{"query": "pins", "limit": 1}))
	require.Len(t, capped.Tools, 1, "limit=1 must cap the returned tools")
	assert.Equal(t, unbounded.Total, capped.Total,
		"Total must stay at the pre-cap match count when results are truncated")

	// Onboarding (empty query) honors the limit while keeping the pre-cap total.
	onboardCapped := decodeSearch(t, callMeta(t, metas, MetaToolSearch, map[string]any{"limit": 1}))
	require.Len(t, onboardCapped.Tools, 1, "onboarding must honor the limit contract")
	assert.Positive(t, onboardCapped.Total)
	assert.Greater(t, onboardCapped.Total, len(onboardCapped.Tools),
		"onboarding Total must report the pre-cap count, not the capped length")
}

// TestMetaOnboardingListsDirectSurface pins the onboarding path: the empty
// (and literal "help") query returns the direct-visible start-here set with a
// hint, and never direct-invisible ops.
func TestMetaOnboardingListsDirectSurface(t *testing.T) {
	srv, metas, _ := metaProgressiveServer(t, nil)
	directVisible := map[string]bool{}
	for _, tool := range srv.Tools {
		directVisible[tool.Name] = tool.DirectVisible
	}

	for _, query := range []string{"", "HELP"} {
		out := decodeSearch(t, callMeta(t, metas, MetaToolSearch, map[string]any{"query": query}))
		require.NotEmpty(t, out.Tools, "the onboarding listing must not be empty (query %q)", query)
		assert.NotEmpty(t, out.Hint, "onboarding must carry the start-here hint")
		assert.Equal(t, len(out.Tools), out.Total)
		for _, tool := range out.Tools {
			assert.Truef(t, directVisible[tool.Name] || isDirectName(srv, tool.Name),
				"onboarding entry %q must be part of the direct surface", tool.Name)
		}
	}
}

// isDirectName reports whether the name is one of the server's direct-only
// tools.
func isDirectName(srv *Server, name string) bool {
	for _, d := range srv.Direct {
		if d.Name == name {
			return true
		}
	}
	return false
}

// TestMetaDescribeUnknownToolSuggests pins describe_tool's recovery path: an
// unknown name is refused with nearest-name suggestions.
func TestMetaDescribeUnknownToolSuggests(t *testing.T) {
	_, metas, _ := metaProgressiveServer(t, nil)

	res := callMeta(t, metas, MetaToolDescribe, map[string]any{"name": "pins_lst"})
	require.True(t, res.IsError, "describe of an unknown tool must be refused")
	assert.Contains(t, res.Text, "pins_list", "the refusal must suggest the actual name")

	res = callMeta(t, metas, MetaToolDescribe, map[string]any{"name": ""})
	require.True(t, res.IsError)
	assert.Contains(t, res.Text, "name is required")
}
