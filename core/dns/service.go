// Package dns provides the DNS zone and record operations for the Pinner
// content-network services. It is defined by the DNSService interface and is
// Output-free: methods return typed results; diagnostics go through an
// injected zap logger.
package dns

import (
	"context"

	ipfs "go.lumeweb.com/ipfs-sdk"
	"go.lumeweb.com/opmesh"
	"go.lumeweb.com/pinner/core/config"
	coreerrors "go.lumeweb.com/pinner/core/errors"
	"go.lumeweb.com/pinner/core/ipfsbase"
	"go.uber.org/zap"
)

// Service defines the interface for DNS operations.
type Service interface {
	RequireAuthenticated() error
	// SetAuthToken hot-updates the auth token on a running service without
	// reconstructing it (used by long-lived consumers on config live-reload).
	SetAuthToken(token string)

	// Zone operations
	CreateZone(ctx context.Context, domain string, nameservers []string) (*ipfs.ZoneResponse, error)
	ListZones(ctx context.Context) ([]ipfs.ZoneListResponse, error)
	// ListZonesPage returns a server-side paged window of the user's zones plus
	// the total zone count. The backend applies a default 10-item window, so an
	// internal scan that must see every zone (e.g. domain-to-zone resolution)
	// has to page through ListZonesPage rather than rely on ListZones.
	ListZonesPage(ctx context.Context, opts opmesh.ListOptions[struct{}]) ([]ipfs.ZoneListResponse, int, error)
	GetZone(ctx context.Context, id string) (*ipfs.ZoneResponse, error)
	DeleteZone(ctx context.Context, id string) error
	ValidateZone(ctx context.Context, id string) (*ipfs.ValidationResponse, error)

	// Record operations
	CreateRecord(ctx context.Context, id string, record ipfs.RecordRequest) (*ipfs.RecordResponse, error)
	ListRecords(ctx context.Context, id string) ([]ipfs.RecordResponse, error)
	// ListRecordsPage returns a server-side paged window of a zone's records
	// plus the total record count. Like ListZonesPage, this is the form that
	// can reach records beyond the first page for an internal scan.
	ListRecordsPage(ctx context.Context, id string, opts opmesh.ListOptions[struct{}]) ([]ipfs.RecordResponse, int, error)
	GetRecord(ctx context.Context, id string, name string, recordType string) (*ipfs.RecordResponse, error)
	UpdateRecord(ctx context.Context, id string, name string, recordType string, record ipfs.RecordRequest) (*ipfs.RecordResponse, error)
	// DeleteRecord deletes a DNS record. When content is provided and non-empty,
	// only the record with that content is deleted; otherwise the whole RRSet is
	// deleted. Multiple contents are rejected.
	DeleteRecord(ctx context.Context, id string, name string, recordType string, content ...string) error
}

// serviceCLI wraps the SDK DNS service with Pinner-specific functionality.
type serviceCLI struct {
	*ipfsbase.Base
	service ipfs.DNSService
	client  *ipfs.Client // injected client (nil = create default)
	log     *zap.Logger
}

// Option is a function that configures a serviceCLI.
type Option func(*serviceCLI)

// WithAuthToken sets an auth token override that takes precedence over config.
func WithAuthToken(token string) Option {
	return func(s *serviceCLI) {
		s.Base.SetAuthTokenOverride(token)
	}
}

// WithClient sets a pre-configured ipfs.Client, bypassing the default
// ipfs.NewClient() call.
func WithClient(client *ipfs.Client) Option {
	return func(s *serviceCLI) {
		s.client = client
	}
}

// New creates a new DNS Service instance with the provided configuration.
// It must NOT copy cfgMgr.Config().AuthToken into the base auth token so
// GetAuthToken() reads config live at request time (live reload for long-lived
// services). Explicit WithAuthToken overrides still take precedence.
// A nil logger is treated as a no-op logger.
func New(cfgMgr config.Manager, apiEndpoint string, logger *zap.Logger, opts ...Option) Service {
	s := &serviceCLI{
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
		s.service = s.client.DNS()
	} else {
		client, err := ipfs.NewClient(apiEndpoint, s.GetAuthToken())
		if err != nil {
			s.log.Debug("could not create dns client", zap.Error(err))
			s.service = nil
			return s
		}
		s.client = client
		s.service = client.DNS()
	}
	return s
}

// SetAuthToken hot-updates the auth token on the retained *ipfs.Client and
// re-fetches the sub-service. No-op when no client is retained.
// The write lock serializes this (config-watcher goroutine) with request reads.
func (s *serviceCLI) SetAuthToken(token string) {
	s.Lock()
	defer s.Unlock()
	if s.client != nil {
		if err := s.client.SetAuthToken(token); err == nil {
			s.service = s.client.DNS()
		}
	}
}

// requireService returns the current sub-service under the read lock, so the
// config-watcher goroutine (SetAuthToken) cannot swap s.service mid-request.
func (s *serviceCLI) requireService() (ipfs.DNSService, error) {
	s.RLock()
	defer s.RUnlock()
	if s.service == nil {
		return nil, coreerrors.ErrServiceUnavailable
	}
	return s.service, nil
}

// CreateZone creates a new DNS zone.
func (s *serviceCLI) CreateZone(ctx context.Context, domain string, nameservers []string) (*ipfs.ZoneResponse, error) {
	if err := s.RequireAuthenticated(); err != nil {
		return nil, err
	}
	svc, err := s.requireService()
	if err != nil {
		return nil, err
	}
	return svc.CreateZone(ctx, domain, nameservers)
}

// ListZonesPage lists a server-side paged window of DNS zones plus the total
// zone count. Paging options (Start/Limit) are forwarded to the SDK so the
// query reaches the backend rather than being truncated to the first page.
func (s *serviceCLI) ListZonesPage(ctx context.Context, opts opmesh.ListOptions[struct{}]) ([]ipfs.ZoneListResponse, int, error) {
	if err := s.RequireAuthenticated(); err != nil {
		return nil, 0, err
	}
	svc, err := s.requireService()
	if err != nil {
		return nil, 0, err
	}
	page, err := svc.ListZonesPage(ctx, ipfs.WithZonesStart(opts.Start), ipfs.WithZonesLimit(opts.Limit))
	if err != nil {
		return nil, 0, err
	}
	return page.Data, page.Total, nil
}

// ListZones lists the backend's default window of DNS zones. Kept for callers
// that only need the first page; domain resolution must use ListZonesPage so a
// zone beyond the first page is not missed.
func (s *serviceCLI) ListZones(ctx context.Context) ([]ipfs.ZoneListResponse, error) {
	zones, _, err := s.ListZonesPage(ctx, opmesh.ListOptions[struct{}]{})
	return zones, err
}

// GetZone retrieves a specific DNS zone.
func (s *serviceCLI) GetZone(ctx context.Context, id string) (*ipfs.ZoneResponse, error) {
	if err := s.RequireAuthenticated(); err != nil {
		return nil, err
	}
	svc, err := s.requireService()
	if err != nil {
		return nil, err
	}
	return svc.GetZone(ctx, id)
}

// DeleteZone deletes a DNS zone.
func (s *serviceCLI) DeleteZone(ctx context.Context, id string) error {
	if err := s.RequireAuthenticated(); err != nil {
		return err
	}
	svc, err := s.requireService()
	if err != nil {
		return err
	}
	return svc.DeleteZone(ctx, id)
}

// ValidateZone validates a DNS zone's nameserver delegation.
func (s *serviceCLI) ValidateZone(ctx context.Context, id string) (*ipfs.ValidationResponse, error) {
	if err := s.RequireAuthenticated(); err != nil {
		return nil, err
	}
	svc, err := s.requireService()
	if err != nil {
		return nil, err
	}
	return svc.ValidateZone(ctx, id)
}

// CreateRecord creates a new DNS record.
func (s *serviceCLI) CreateRecord(ctx context.Context, id string, record ipfs.RecordRequest) (*ipfs.RecordResponse, error) {
	if err := s.RequireAuthenticated(); err != nil {
		return nil, err
	}
	svc, err := s.requireService()
	if err != nil {
		return nil, err
	}
	return svc.CreateRecord(ctx, id, record)
}

// ListRecordsPage lists a server-side paged window of a zone's DNS records
// plus the total record count. Paging options (Start/Limit) are forwarded to
// the SDK so the query reaches the backend rather than being truncated to the
// first page.
func (s *serviceCLI) ListRecordsPage(ctx context.Context, id string, opts opmesh.ListOptions[struct{}]) ([]ipfs.RecordResponse, int, error) {
	if err := s.RequireAuthenticated(); err != nil {
		return nil, 0, err
	}
	svc, err := s.requireService()
	if err != nil {
		return nil, 0, err
	}
	page, err := svc.ListRecordsPage(ctx, id, ipfs.WithRecordsStart(opts.Start), ipfs.WithRecordsLimit(opts.Limit))
	if err != nil {
		return nil, 0, err
	}
	return page.Data, page.Total, nil
}

// ListRecords lists the backend's default window of DNS records for a zone.
// Kept for callers that only need the first page; single-record delete
// resolution must use ListRecordsPage so a record beyond the first page is not
// silently considered absent.
func (s *serviceCLI) ListRecords(ctx context.Context, id string) ([]ipfs.RecordResponse, error) {
	records, _, err := s.ListRecordsPage(ctx, id, opmesh.ListOptions[struct{}]{})
	return records, err
}

// GetRecord retrieves a specific DNS record.
func (s *serviceCLI) GetRecord(ctx context.Context, id string, name string, recordType string) (*ipfs.RecordResponse, error) {
	if err := s.RequireAuthenticated(); err != nil {
		return nil, err
	}
	svc, err := s.requireService()
	if err != nil {
		return nil, err
	}
	return svc.GetRecord(ctx, id, name, recordType)
}

// UpdateRecord updates a DNS record.
func (s *serviceCLI) UpdateRecord(ctx context.Context, id string, name string, recordType string, record ipfs.RecordRequest) (*ipfs.RecordResponse, error) {
	if err := s.RequireAuthenticated(); err != nil {
		return nil, err
	}
	svc, err := s.requireService()
	if err != nil {
		return nil, err
	}
	return svc.UpdateRecord(ctx, id, name, recordType, record)
}

// DeleteRecord deletes a DNS record. When content is provided and non-empty, only
// the record with that content is deleted; otherwise the whole RRSet is deleted.
func (s *serviceCLI) DeleteRecord(ctx context.Context, id string, name string, recordType string, content ...string) error {
	if err := s.RequireAuthenticated(); err != nil {
		return err
	}
	svc, err := s.requireService()
	if err != nil {
		return err
	}
	return svc.DeleteRecord(ctx, id, name, recordType, content...)
}
