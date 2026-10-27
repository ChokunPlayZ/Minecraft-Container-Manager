package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

const digitalOceanAPI = "https://api.digitalocean.com/v2"

type digitalOceanProvider struct {
	cfg  *config
	http *http.Client
}

type doRecord struct {
	ID       int64  `json:"id,omitempty"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Data     string `json:"data"`
	Port     int    `json:"port"`
	TTL      int    `json:"ttl"`
	Priority int    `json:"priority"`
	Weight   int    `json:"weight"`
}

func (p *digitalOceanProvider) path() string { return "/domains/" + url.PathEscape(p.cfg.Zone) }

func (p *digitalOceanProvider) upsert(ctx context.Context, current, desired *Record) (string, error) {
	name, err := relativeName(desired.Name, p.cfg.Zone)
	if err != nil {
		return "", err
	}
	body := doRecord{Type: "SRV", Name: name, Data: desired.Target, Port: desired.Port, TTL: desired.TTL, Priority: desired.Priority, Weight: desired.Weight}
	var result struct {
		Record doRecord `json:"domain_record"`
	}
	if current != nil && current.RecordID != "" {
		err := p.do(ctx, http.MethodPut, p.path()+"/records/"+url.PathEscape(current.RecordID), body, &result)
		if err == nil {
			return current.RecordID, nil
		}
		if !recordNotFound(err) {
			return "", err
		}
	}
	var found struct {
		Records []doRecord `json:"domain_records"`
	}
	query := url.Values{"type": {"SRV"}, "name": {desired.Name}, "per_page": {"200"}}
	if err := p.do(ctx, http.MethodGet, p.path()+"/records?"+query.Encode(), nil, &found); err != nil {
		return "", err
	}
	if len(found.Records) > 0 {
		id := strconv.FormatInt(found.Records[0].ID, 10)
		return id, p.do(ctx, http.MethodPut, p.path()+"/records/"+id, body, &result)
	}
	if err := p.do(ctx, http.MethodPost, p.path()+"/records", body, &result); err != nil {
		return "", err
	}
	if result.Record.ID <= 0 {
		return "", fmt.Errorf("DigitalOcean returned no record ID")
	}
	return strconv.FormatInt(result.Record.ID, 10), nil
}

func (p *digitalOceanProvider) remove(ctx context.Context, rec *Record) error {
	err := p.do(ctx, http.MethodDelete, p.path()+"/records/"+url.PathEscape(rec.RecordID), nil, nil)
	if recordNotFound(err) {
		return nil
	}
	return err
}

func (p *digitalOceanProvider) verify(ctx context.Context) (*zoneInfo, error) {
	var result struct {
		Domain struct {
			Name string `json:"name"`
		} `json:"domain"`
	}
	if err := p.do(ctx, http.MethodGet, p.path(), nil, &result); err != nil {
		return nil, err
	}
	return &zoneInfo{Name: result.Domain.Name, Status: "active"}, nil
}

func (p *digitalOceanProvider) do(ctx context.Context, method, path string, body, result any) error {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, digitalOceanAPI+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var failure struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &failure)
		return &providerError{resp.StatusCode, fmt.Sprintf("DigitalOcean API (%d): %s", resp.StatusCode, failure.Message)}
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(raw, result)
}
