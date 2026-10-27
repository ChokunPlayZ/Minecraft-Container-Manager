package dns

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Each provider owns remote operations; address preferences and tracking stay
// in Service so they have the same lifecycle regardless of DNS host.
type provider interface {
	upsert(context.Context, *Record, *Record) (string, error)
	remove(context.Context, *Record) error
	verify(context.Context) (*zoneInfo, error)
}

func (s *Service) provider(cfg *config) (provider, error) {
	switch cfg.Provider {
	case "cloudflare":
		return &cloudflareProvider{newCFClient(cfg.APIToken, cfg.Zone, s.http)}, nil
	case "digitalocean":
		return &digitalOceanProvider{cfg: cfg, http: s.http}, nil
	case "porkbun":
		return &porkbunProvider{cfg: cfg, http: s.http}, nil
	case "route53":
		return &route53Provider{cfg: cfg, http: s.http}, nil
	default:
		return nil, fmt.Errorf("unsupported DNS provider %q", cfg.Provider)
	}
}

type providerError struct {
	status  int
	message string
}

func (e *providerError) Error() string { return e.message }

func recordNotFound(err error) bool {
	var apiErr *providerError
	return errors.As(err, &apiErr) && apiErr.status == http.StatusNotFound
}

func relativeName(name, zone string) (string, error) {
	name = strings.TrimSuffix(strings.ToLower(name), ".")
	zone = strings.TrimSuffix(strings.ToLower(zone), ".")
	if !strings.HasSuffix(name, "."+zone) {
		return "", fmt.Errorf("record %q does not belong to zone %q", name, zone)
	}
	return strings.TrimSuffix(name, "."+zone), nil
}

type cloudflareProvider struct{ *cfClient }

func (p *cloudflareProvider) upsert(ctx context.Context, current, desired *Record) (string, error) {
	parts := strings.SplitN(desired.Name, ".", 3)
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid SRV record name %q", desired.Name)
	}
	update := func(id string) error {
		return p.updateRecord(ctx, id, parts[0], parts[1], parts[2], desired.Name, desired.Target, desired.Port, desired.TTL, desired.Priority, desired.Weight)
	}
	if current != nil && current.RecordID != "" {
		err := update(current.RecordID)
		if err == nil {
			return current.RecordID, nil
		}
		if !recordNotFound(err) {
			return "", err
		}
	}
	id, err := p.findSRVRecord(ctx, desired.Name)
	if err != nil {
		return "", err
	}
	if id != "" {
		return id, update(id)
	}
	return p.createRecord(ctx, parts[0], parts[1], parts[2], desired.Name, desired.Target, desired.Port, desired.TTL, desired.Priority, desired.Weight)
}

func (p *cloudflareProvider) remove(ctx context.Context, rec *Record) error {
	err := p.deleteRecord(ctx, rec.RecordID)
	if recordNotFound(err) {
		return nil
	}
	return err
}

func (p *cloudflareProvider) verify(ctx context.Context) (*zoneInfo, error) {
	return p.verifyZone(ctx)
}
