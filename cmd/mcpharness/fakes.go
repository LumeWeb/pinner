package main

// fakes.go hand-fakes pinner's core services so the assembled
// assembly.AssembleCatalogOps bundle works fully IN-PROCESS (never touching the
// network). Every fake implements only the narrow method set the catalogops
// handlers actually call, keeping deterministic in-memory state (one seeded
// pin, one seeded IPNS key "seed-key", one seeded DNS zone, one seeded
// website, one seeded API key). Methods the harness surface never reaches
// panic on the embedded nil interface — mirrors the accepted test pattern of
// "embed the real interface and override only what's needed".

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	ipfs "go.lumeweb.com/ipfs-sdk"

	portalsdk "go.lumeweb.com/portal-sdk"

	"go.lumeweb.com/pinner/core/apikeys"
	"go.lumeweb.com/pinner/core/auth"
	"go.lumeweb.com/pinner/core/config"
	"go.lumeweb.com/pinner/core/dns"
	"go.lumeweb.com/pinner/core/ipns"
	"go.lumeweb.com/pinner/core/operations"
	"go.lumeweb.com/pinner/core/pinning"
	"go.lumeweb.com/pinner/core/websites"
)

// fakeBaseEndpoint is the loopback "Portal API" the fakes pretend to be. No
// network request is ever made; the value only shows up in results
// (portal_url, deep-links) and config reads.
const fakeBaseEndpoint = "http://127.0.0.1:8126"

// fakePortalTime is the stable timestamp stamped onto seeded records so the
// fakes' output is deterministic across runs.
var fakePortalTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// FakeServices holds the in-memory faked core services shared by the lazy dep
// closures built in buildCatalogDepsBundle. Each domain handler constructs a
// fresh fake-per-invocation through the deps getters, but they all share these
// underlying stores, so mutations (a pins_add / pins_delete round trip, a
// new IPNS key) are visible to later operations in the same session.
type FakeServices struct {
	// Email is the seeded account identity and (via tokenForEmail) the auth
	// token value the config manager stores.
	Email string
	// Token is the HTTP-bearer the HTTP mode middleware validates against;
	// it is also written into the config manager so ResolveAuthToken sees it.
	Token string

	// cfgMgr is the temp-file-backed pinner config manager.
	cfgMgr config.Manager

	// mu guards the mutable stores below. The fakes are invoked from MCP tool
	// handlers that may run concurrently (streamable HTTP + app helpers).
	pins     map[string]pinning.Pin
	pinOrder []string

	keys map[string]ipfs.IPNSKeyResponse

	zones map[string]ipfs.ZoneListResponse

	websites map[string]ipfs.WebsiteItem

	// apiKeys is keyed by key name; the APIKey struct's promoted (embedded)
	// generated fields cannot be set in a composite literal, so identity is
	// the name, not the UUID.
	apiKeys map[string]*portalsdk.APIKey
}

// fakeError is the uniform "not implemented by the harness fake" error.
func fakeError(what string) error {
	return fmt.Errorf("harness fake: %s is not implemented (test-only in-memory server)", what)
}

// CfgMgr returns the shared config manager (the lazy-deps getter).
func (f *FakeServices) CfgMgr() config.Manager { return f.cfgMgr }
func (f *FakeServices) secure() bool           { return false } // loopback endpoint is plain http
func (f *FakeServices) resolveToken(m config.Manager) string {
	if m == nil {
		return f.Token
	}
	if t := m.Config().AuthToken; t != "" {
		return t
	}
	return f.Token
}

// NewFakeServices builds the fake service cluster: a real (temp-file-backed)
// config manager seeded with BaseEndpoint/AuthToken/Secure=false, plus the
// deterministic in-memory domain stores.
func NewFakeServices(email string) (*FakeServices, error) {
	dir, err := os.MkdirTemp("", "mcpharness")
	if err != nil {
		return nil, fmt.Errorf("fakes: temp config dir: %w", err)
	}
	cfgPath := filepath.Join(dir, "config.yaml")
	mgr, err := config.NewManager(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("fakes: config manager: %w", err)
	}
	token := "token-" + email
	if err := mgr.SetBaseEndpoint(fakeBaseEndpoint); err != nil {
		return nil, fmt.Errorf("fakes: set base endpoint: %w", err)
	}
	if err := mgr.SetAuthToken(token); err != nil {
		return nil, fmt.Errorf("fakes: set auth token: %w", err)
	}
	if err := mgr.SetSecure(false); err != nil {
		return nil, fmt.Errorf("fakes: set secure: %w", err)
	}
	_ = mgr.Save() // best effort; the in-memory view is already correct

	f := &FakeServices{
		Email:  email,
		Token:  token,
		cfgMgr: mgr,
		pins: map[string]pinning.Pin{
			"bafybeihrandomseedcid0000000000000000000000000000": {
				CID:     "bafybeihrandomseedcid0000000000000000000000000000",
				Name:    "seed-pin",
				Status:  "pinned",
				Created: fakePortalTime.Format(time.RFC3339),
			},
		},
		pinOrder: []string{"bafybeihrandomseedcid0000000000000000000000000000"},
		keys: map[string]ipfs.IPNSKeyResponse{
			"seed-key": {
				Id:       1,
				Name:     "seed-key",
				IpnsName: "k51qzi5uqu5dharnessseedkey0000000000000000000000",
				PeerId:   "12D3KooWFakePeerIdHarnessSeedKey00000000000",
				Created:  fakePortalTime,
			},
		},
		zones: map[string]ipfs.ZoneListResponse{
			"example.com": {
				Id:        1,
				Domain:    "example.com",
				Status:    "active",
				UserId:    1,
				CreatedAt: fakePortalTime,
				UpdatedAt: fakePortalTime,
			},
		},
		websites: map[string]ipfs.WebsiteItem{
			"seed.example.com": {
				Id:         1,
				Domain:     "seed.example.com",
				TargetHash: "bafybeihrandomseedcid0000000000000000000000000000",
				TargetType: "ipfs",
				Status:     "active",
				Created:    fakePortalTime,
				Updated:    fakePortalTime,
			},
		},
	}

	// Seed one API key. portal-sdk's AccountInfo/APIKey types alias generated
	// structs through embedding, whose promoted fields cannot be set in a
	// composite literal — the public NewAPIKey constructor is the only seam.
	f.apiKeys = map[string]*portalsdk.APIKey{
		"seed-key": portalsdk.NewAPIKey("seed-key", "fake-apikey-seed"),
	}

	return f, nil
}

// --- auth.AuthService fake -------------------------------------------------

// fakeAuthService satisfies exactly the auth.AuthService surface the
// catalogops auth/account handlers call: Status and GetAccount.
type fakeAuthService struct {
	auth.AuthService
	svc *FakeServices
}

func newFakeAuthService(svc *FakeServices) auth.AuthService { return &fakeAuthService{svc: svc} }

func (f *fakeAuthService) Status(context.Context) (*auth.StatusResult, error) {
	return &auth.StatusResult{PortalURL: fakeBaseEndpoint}, nil
}

func (f *fakeAuthService) GetAccount(context.Context) (*portalsdk.AccountInfo, error) {
	// Promoted (embedded) fields cannot appear in a composite literal; assign.
	acct := &portalsdk.AccountInfo{}
	acct.Email = f.svc.Email
	acct.Id = 1
	acct.Verified = true
	return acct, nil
}

// --- pinning.PinningService fake -------------------------------------------

// fakePinningService satisfies the pin surface the pins_* operations use.
type fakePinningService struct {
	pinning.PinningService
	svc *FakeServices
}

func newFakePinningService(svc *FakeServices) pinning.PinningService {
	return &fakePinningService{svc: svc}
}

func (f *fakePinningService) RequireAuthenticated() error { return nil }

func (f *fakePinningService) List(_ context.Context, opts pinning.ListOptions) ([]pinning.Pin, error) {
	out := make([]pinning.Pin, 0, len(f.svc.pinOrder))
	for _, cid := range f.svc.pinOrder {
		p, ok := f.svc.pins[cid]
		if !ok {
			continue
		}
		if opts.Search != "" && !strings.Contains(strings.ToLower(p.Name), strings.ToLower(opts.Search)) {
			continue
		}
		if opts.Name != "" && p.Name != opts.Name {
			continue
		}
		if opts.Status != "" && p.Status != opts.Status {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func (f *fakePinningService) Pin(_ context.Context, cid, name string, _ bool) (*pinning.PinResult, error) {
	if name == "" {
		name = "pin-" + cid
	}
	f.svc.pins[cid] = pinning.Pin{
		CID:     cid,
		Name:    name,
		Status:  "pinned",
		Created: fakePortalTime.Format(time.RFC3339),
	}
	f.svc.pinOrder = append(f.svc.pinOrder, cid)
	return &pinning.PinResult{CID: cid, Status: "pinned", RequestID: "fake-" + name}, nil
}

func (f *fakePinningService) Status(_ context.Context, cid string, _ bool) (*pinning.PinStatus, error) {
	p, ok := f.svc.pins[cid]
	if !ok {
		return nil, fmt.Errorf("pin %q not found", cid)
	}
	return &pinning.PinStatus{CID: cid, Status: p.Status, Created: p.Created}, nil
}

func (f *fakePinningService) Unpin(_ context.Context, cid string, _ bool) (*pinning.UnpinResult, error) {
	if _, ok := f.svc.pins[cid]; !ok {
		return nil, fmt.Errorf("pin %q not found", cid)
	}
	delete(f.svc.pins, cid)
	return &pinning.UnpinResult{CID: cid}, nil
}

func (f *fakePinningService) PinBatch(ctx context.Context, cids []string, name string, _ pinning.BatchOptions) (*pinning.BatchResult, error) {
	for _, cid := range cids {
		if _, err := f.Pin(ctx, cid, name, false); err != nil {
			continue
		}
	}
	return &pinning.BatchResult{Total: len(cids)}, nil
}

func (f *fakePinningService) UnpinBatch(_ context.Context, cids []string, _ pinning.BatchOptions) (*pinning.BatchResult, error) {
	for _, cid := range cids {
		_, _ = f.Unpin(context.Background(), cid, true)
	}
	return &pinning.BatchResult{Total: len(cids)}, nil
}

func (f *fakePinningService) UnpinAll(context.Context, string, pinning.BatchOptions) (*pinning.BatchResult, error) {
	n := len(f.svc.pins)
	f.svc.pins = map[string]pinning.Pin{}
	f.svc.pinOrder = nil
	return &pinning.BatchResult{Total: n}, nil
}

func (f *fakePinningService) UpdateMetadata(context.Context, string, []string, bool) error {
	return nil
}

func (f *fakePinningService) UpdatePin(context.Context, string, string, []string, bool) error {
	return nil
}

// --- dns.Service fake --------------------------------------------------------

// fakeDNSService satisfies the zone/record surface the dns_* operations use.
type fakeDNSService struct {
	dns.Service
	svc *FakeServices
}

func newFakeDNSService(svc *FakeServices) dns.Service { return &fakeDNSService{svc: svc} }

func (f *fakeDNSService) RequireAuthenticated() error { return nil }

func (f *fakeDNSService) ListZones(context.Context) ([]ipfs.ZoneListResponse, error) {
	out := make([]ipfs.ZoneListResponse, 0, len(f.svc.zones))
	seen := map[int]bool{}
	for _, z := range f.svc.zones {
		if seen[z.Id] {
			continue
		}
		seen[z.Id] = true
		out = append(out, z)
	}
	return out, nil
}

func (f *fakeDNSService) CreateZone(_ context.Context, domain string, _ []string) (*ipfs.ZoneResponse, error) {
	zone := ipfs.ZoneListResponse{
		Id:        len(f.svc.zones) + 2,
		Domain:    domain,
		Status:    "active",
		UserId:    1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	f.svc.zones[domain] = zone
	return zoneToResponse(zone), nil
}

func (f *fakeDNSService) GetZone(_ context.Context, id string) (*ipfs.ZoneResponse, error) {
	z, ok := f.svc.zones[id]
	if !ok {
		return nil, fmt.Errorf("zone %q not found", id)
	}
	return zoneToResponse(z), nil
}

func (f *fakeDNSService) DeleteZone(context.Context, string) error { return nil }

func (f *fakeDNSService) ValidateZone(_ context.Context, id string) (*ipfs.ValidationResponse, error) {
	if _, ok := f.svc.zones[id]; !ok {
		return nil, fmt.Errorf("zone %q not found", id)
	}
	return &ipfs.ValidationResponse{Valid: true, Message: "harness fake: zone validated", CheckedAt: time.Now()}, nil
}

func (f *fakeDNSService) CreateRecord(_ context.Context, id string, rec ipfs.RecordRequest) (*ipfs.RecordResponse, error) {
	return &ipfs.RecordResponse{
		Id:      "rec-fake-" + rec.Name,
		ZoneId:  parseFakeID(id),
		Name:    rec.Name,
		Type:    rec.Type,
		Content: rec.Content,
	}, nil
}

func (f *fakeDNSService) ListRecords(context.Context, string) ([]ipfs.RecordResponse, error) {
	return []ipfs.RecordResponse{}, nil
}

func (f *fakeDNSService) SetAuthToken(string) {}

func zoneToResponse(z ipfs.ZoneListResponse) *ipfs.ZoneResponse {
	return &ipfs.ZoneResponse{
		Id:        z.Id,
		Domain:    z.Domain,
		Status:    z.Status,
		UserId:    z.UserId,
		CreatedAt: z.CreatedAt,
		UpdatedAt: z.UpdatedAt,
	}
}

func parseFakeID(id string) int {
	n := 0
	for _, r := range id {
		if r < '0' || r > '9' {
			return 1 // stable non-zero for composite ids
		}
		n = n*10 + int(r-'0')
	}
	if n == 0 {
		n = 1
	}
	return n
}

// --- ipns.Service fake -------------------------------------------------------

// fakeIPNSService satisfies the key/publish surface the ipns_* and ens_*
// operations use.
type fakeIPNSService struct {
	ipns.Service
	svc *FakeServices
}

func newFakeIPNSService(svc *FakeServices) ipns.Service { return &fakeIPNSService{svc: svc} }

func (f *fakeIPNSService) RequireAuthenticated() error { return nil }

func (f *fakeIPNSService) ListKeys(_ context.Context, opts ...ipfs.ListKeyOption) ([]ipfs.IPNSKeyResponse, error) {
	filter := ""
	for _, o := range opts {
		if o.FilterName != "" {
			filter = o.FilterName
		}
	}
	out := make([]ipfs.IPNSKeyResponse, 0, len(f.svc.keys))
	keys := sortedKeys(f.svc.keys)
	for _, name := range keys {
		k := f.svc.keys[name]
		if filter != "" && !strings.Contains(k.Name, filter) {
			continue
		}
		out = append(out, k)
	}
	return out, nil
}

func (f *fakeIPNSService) CreateKey(_ context.Context, name string, _ *string) (*ipfs.IPNSKeyResponse, error) {
	if _, exists := f.svc.keys[name]; exists {
		return nil, fmt.Errorf("key %q already exists", name)
	}
	k := ipfs.IPNSKeyResponse{
		Id:       len(f.svc.keys) + 2,
		Name:     name,
		IpnsName: "k51qzi5uqu5dharness" + name,
		PeerId:   "12D3KooWFakePeerIdHarness" + name,
		Created:  time.Now(),
	}
	f.svc.keys[name] = k
	return &k, nil
}

func (f *fakeIPNSService) GetKey(_ context.Context, id string) (*ipfs.IPNSKeyResponse, error) {
	k, ok := f.svc.keys[id]
	if !ok {
		return nil, fmt.Errorf("key %q not found", id)
	}
	return &k, nil
}

func (f *fakeIPNSService) DeleteKey(_ context.Context, id string) error {
	if _, ok := f.svc.keys[id]; !ok {
		return fmt.Errorf("key %q not found", id)
	}
	delete(f.svc.keys, id)
	return nil
}

func (f *fakeIPNSService) Publish(_ context.Context, _, keyName string, _ *string) (*ipfs.IPNSPublishResponse, error) {
	k, ok := f.svc.keys[keyName]
	if !ok {
		return nil, fmt.Errorf("key %q not found", keyName)
	}
	now := time.Now()
	return &ipfs.IPNSPublishResponse{
		Name:      k.IpnsName,
		Published: now,
		Validity:  now.Add(24 * time.Hour),
		Sequence:  1,
		Value:     "fake://" + keyName,
	}, nil
}

func (f *fakeIPNSService) Republish(context.Context, string) (*ipfs.IPNSRepublishResponse, error) {
	return &ipfs.IPNSRepublishResponse{Count: 0, Message: "harness fake: republish"}, nil
}

func (f *fakeIPNSService) Resolve(_ context.Context, name string) (*ipfs.IPNSResolveResponse, error) {
	return &ipfs.IPNSResolveResponse{
		Name:  name,
		Value: "/ipfs/bafybeihrandomseedcid0000000000000000000000000000",
		Path:  "/ipfs/bafybeihrandomseedcid0000000000000000000000000000",
	}, nil
}

func (f *fakeIPNSService) SetAuthToken(string) {}

func sortedKeys(m map[string]ipfs.IPNSKeyResponse) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// --- websites.Service fake ---------------------------------------------------

// fakeWebsitesService satisfies the website CRUD surface the websites_*
// operations use.
type fakeWebsitesService struct {
	websites.Service
	svc *FakeServices
}

func newFakeWebsitesService(svc *FakeServices) websites.Service {
	return &fakeWebsitesService{svc: svc}
}

func (f *fakeWebsitesService) RequireAuthenticated() error { return nil }

func (f *fakeWebsitesService) List(context.Context, websites.ListOptions) ([]ipfs.WebsiteItem, error) {
	out := make([]ipfs.WebsiteItem, 0, len(f.svc.websites))
	domains := make([]string, 0, len(f.svc.websites))
	for d := range f.svc.websites {
		domains = append(domains, d)
	}
	sortStrings(domains)
	for _, d := range domains {
		out = append(out, f.svc.websites[d])
	}
	return out, nil
}

func (f *fakeWebsitesService) Create(_ context.Context, domain, targetHash, targetType string) (*ipfs.WebsiteItem, error) {
	now := time.Now()
	w := ipfs.WebsiteItem{
		Id:         len(f.svc.websites) + 2,
		Domain:     domain,
		TargetHash: targetHash,
		TargetType: targetType,
		Status:     "pending",
		Created:    now,
		Updated:    now,
	}
	f.svc.websites[domain] = w
	return &w, nil
}

func (f *fakeWebsitesService) Get(_ context.Context, id string) (*ipfs.WebsiteItem, error) {
	w, ok := f.svc.websites[id]
	if !ok {
		return nil, fmt.Errorf("website %q not found", id)
	}
	return &w, nil
}

func (f *fakeWebsitesService) Delete(context.Context, string) error { return nil }

func (f *fakeWebsitesService) SetAuthToken(string) {}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// --- operations.Service fake --------------------------------------------------

// fakeOperationsService satisfies the operations.Service surface the
// operations_list / operations_get tools use.
type fakeOperationsService struct {
	operations.Service
	svc *FakeServices
}

func (f *fakeOperationsService) RequireAuthenticated() error { return nil }

func (f *fakeOperationsService) List(context.Context, operations.ListOptions) (*operations.OperationsListResult, error) {
	return &operations.OperationsListResult{Operations: []operations.OperationListItem{}, Total: 0}, nil
}

func (f *fakeOperationsService) Get(context.Context, int64) (*operations.OperationDetail, error) {
	return nil, fakeError("operations_get")
}

func (f *fakeOperationsService) Watch(ctx context.Context, id int64) (*operations.OperationDetail, error) {
	return f.Get(ctx, id)
}

// --- apikeys.Service fake -------------------------------------------------------

// fakeAPIKeysService satisfies the apikeys surface the api_keys_* tools use.
type fakeAPIKeysService struct {
	apikeys.Service
	svc *FakeServices
}

func (f *fakeAPIKeysService) RequireAuthenticated() error { return nil }

func (f *fakeAPIKeysService) ListAPIKeys(context.Context, string) ([]*portalsdk.APIKey, int, error) {
	names := make([]string, 0, len(f.svc.apiKeys))
	for name := range f.svc.apiKeys {
		names = append(names, name)
	}
	sortStrings(names)
	out := make([]*portalsdk.APIKey, 0, len(names))
	for _, name := range names {
		out = append(out, f.svc.apiKeys[name])
	}
	return out, len(out), nil
}

func (f *fakeAPIKeysService) CreateAPIKey(_ context.Context, name string) (*portalsdk.APIKey, error) {
	if _, exists := f.svc.apiKeys[name]; exists {
		return nil, fmt.Errorf("api key %q already exists", name)
	}
	k := portalsdk.NewAPIKey(name, "fake-apikey-"+name)
	f.svc.apiKeys[name] = k
	return k, nil
}

func (f *fakeAPIKeysService) DeleteAPIKey(_ context.Context, idOrName string, _ bool) error {
	k, ok := f.svc.apiKeys[idOrName]
	if !ok {
		return fmt.Errorf("api key %q not found", idOrName)
	}
	_ = k
	delete(f.svc.apiKeys, idOrName)
	return nil
}

func (f *fakeAPIKeysService) GetCurrentAPIKeyUUID() string { return "" }

// fakeAPIKeyDrop is the one-time OOB hand-off seam api_keys_create uses in
// model-facing assemblies. The harness stores the value in memory and returns
// a never-served loopback drop URL.
type fakeAPIKeyDrop struct{}

func (fakeAPIKeyDrop) Drop(_ context.Context, key *portalsdk.APIKey) (string, error) {
	if key == nil {
		return "", fakeError("api key drop: nil key")
	}
	return fakeBaseEndpoint + "/drop/" + key.Uuid.String(), nil
}
