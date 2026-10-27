package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const porkbunAPI = "https://api.porkbun.com/api/json/v3"

type porkbunProvider struct {
	cfg  *config
	http *http.Client
}

type porkbunResponse struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	ID      json.RawMessage `json:"id"`
	Records []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"records"`
}

func (p *porkbunProvider) upsert(ctx context.Context, current, desired *Record) (string, error) {
	name, err := relativeName(desired.Name, p.cfg.Zone)
	if err != nil {
		return "", err
	}
	// Porkbun keeps priority separate; content contains weight, port, target.
	body := map[string]any{"name": name, "type": "SRV", "content": fmt.Sprintf("%d %d %s", desired.Weight, desired.Port, desired.Target), "ttl": desired.TTL, "prio": desired.Priority}
	zone := url.PathEscape(p.cfg.Zone)
	if current != nil && current.RecordID != "" {
		_, err := p.do(ctx, "/dns/edit/"+zone+"/"+url.PathEscape(current.RecordID), body)
		if err == nil {
			return current.RecordID, nil
		}
		if !recordNotFound(err) {
			return "", err
		}
	}
	found, err := p.do(ctx, "/dns/retrieveByNameType/"+zone+"/SRV/"+url.PathEscape(name), nil)
	if err != nil {
		return "", err
	}
	if len(found.Records) > 0 {
		id := found.Records[0].ID
		_, err := p.do(ctx, "/dns/edit/"+zone+"/"+url.PathEscape(id), body)
		return id, err
	}
	created, err := p.do(ctx, "/dns/create/"+zone, body)
	if err != nil {
		return "", err
	}
	// API versions have returned both JSON strings and numbers for new IDs.
	id := strings.Trim(string(created.ID), `"`)
	if id == "" || id == "null" {
		return "", fmt.Errorf("Porkbun returned no record ID")
	}
	return id, nil
}

func (p *porkbunProvider) remove(ctx context.Context, rec *Record) error {
	_, err := p.do(ctx, "/dns/delete/"+url.PathEscape(p.cfg.Zone)+"/"+url.PathEscape(rec.RecordID), nil)
	if recordNotFound(err) {
		return nil
	}
	return err
}

func (p *porkbunProvider) verify(ctx context.Context) (*zoneInfo, error) {
	_, err := p.do(ctx, "/dns/retrieve/"+url.PathEscape(p.cfg.Zone), nil)
	if err != nil {
		return nil, err
	}
	return &zoneInfo{Name: p.cfg.Zone, Status: "active"}, nil
}

func (p *porkbunProvider) do(ctx context.Context, path string, body map[string]any) (*porkbunResponse, error) {
	if body == nil {
		body = map[string]any{}
	}
	body["apikey"] = p.cfg.APIToken
	body["secretapikey"] = p.cfg.APISecret
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, porkbunAPI+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result porkbunResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Porkbun response (%d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || result.Status != "SUCCESS" {
		status := resp.StatusCode
		message := strings.ToLower(result.Message)
		if strings.Contains(message, "record") && (strings.Contains(message, "not found") || strings.Contains(message, "does not exist") || strings.Contains(message, "invalid record id")) {
			status = http.StatusNotFound
		}
		return nil, &providerError{status, fmt.Sprintf("Porkbun API (%d): %s", resp.StatusCode, result.Message)}
	}
	return &result, nil
}
