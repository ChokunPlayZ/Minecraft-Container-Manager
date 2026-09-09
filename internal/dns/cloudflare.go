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
	"time"
)

// cloudflareAPI is the base URL for the Cloudflare v4 REST API.
const cloudflareAPI = "https://api.cloudflare.com/client/v4"

// defaultClientTimeout applies when a caller does not supply an HTTP client.
const defaultClientTimeout = 15 * time.Second

// cfClient talks to the Cloudflare DNS Records API for a single zone.
type cfClient struct {
	token  string
	zone   string
	http   *http.Client
	apiURL string
}

// newCFClient builds a Cloudflare client bound to a zone. testURL, when
// non-empty, overrides the default API base URL for tests.
func newCFClient(token, zone string, hc *http.Client, testURL ...string) *cfClient {
	if hc == nil {
		hc = &http.Client{}
	}
	base := cloudflareAPI
	if len(testURL) > 0 && testURL[0] != "" {
		base = testURL[0]
	}
	if hc.Timeout == 0 {
		hc.Timeout = defaultClientTimeout
	}
	return &cfClient{token: token, zone: zone, http: hc, apiURL: base}
}

// cfSRVData holds the structured SRV fields required by Cloudflare API v4.
type cfSRVData struct {
	Service  string `json:"service"`
	Proto    string `json:"proto"`
	Name     string `json:"name"`
	Priority int    `json:"priority"`
	Weight   int    `json:"weight"`
	Port     int    `json:"port"`
	Target   string `json:"target"`
}

// cfSRVRecord is the request/response body for an SRV DNS record.
type cfSRVRecord struct {
	Type string    `json:"type"`
	Name string    `json:"name"`
	TTL  int       `json:"ttl"`
	Data cfSRVData `json:"data"`
}

// cfZoneInfo contains zone metadata returned by Cloudflare.
type cfZoneInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// cfResponse is the common Cloudflare API envelope.
type cfResponse struct {
	Success bool             `json:"success"`
	Result  *json.RawMessage `json:"result"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

// createRecord creates a new SRV record and returns its ID.
func (c *cfClient) createRecord(ctx context.Context, service, proto, srvName, recordName, target string, port, ttl, priority, weight int) (string, error) {
	body, err := c.recordPayload(service, proto, srvName, recordName, target, port, ttl, priority, weight)
	if err != nil {
		return "", err
	}
	raw, err := c.do(ctx, http.MethodPost, "/zones/"+c.zone+"/dns_records", body)
	if err != nil {
		return "", err
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("decode result: %w", err)
	}
	return result.ID, nil
}

// updateRecord updates an existing SRV record in place.
func (c *cfClient) updateRecord(ctx context.Context, recordID, service, proto, srvName, recordName, target string, port, ttl, priority, weight int) error {
	body, err := c.recordPayload(service, proto, srvName, recordName, target, port, ttl, priority, weight)
	if err != nil {
		return err
	}
	_, err = c.do(ctx, http.MethodPut, "/zones/"+c.zone+"/dns_records/"+recordID, body)
	return err
}

// deleteRecord removes an existing SRV record.
func (c *cfClient) deleteRecord(ctx context.Context, recordID string) error {
	_, err := c.do(ctx, http.MethodDelete, "/zones/"+c.zone+"/dns_records/"+recordID, nil)
	return err
}

// findSRVRecord searches for an existing SRV record with the exact name in the zone.
// Returns the record ID if found, or an empty string if none exists.
func (c *cfClient) findSRVRecord(ctx context.Context, name string) (string, error) {
	query := url.Values{}
	query.Set("type", "SRV")
	query.Set("name", name)
	raw, err := c.do(ctx, http.MethodGet, "/zones/"+c.zone+"/dns_records?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	var results []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &results); err != nil {
		return "", fmt.Errorf("decode find result: %w", err)
	}
	if len(results) > 0 {
		return results[0].ID, nil
	}
	return "", nil
}

// verifyZone verifies that the configured API token has access to the zone.
func (c *cfClient) verifyZone(ctx context.Context) (*cfZoneInfo, error) {
	raw, err := c.do(ctx, http.MethodGet, "/zones/"+c.zone, nil)
	if err != nil {
		return nil, err
	}
	var zone cfZoneInfo
	if err := json.Unmarshal(raw, &zone); err != nil {
		return nil, fmt.Errorf("decode zone result: %w", err)
	}
	return &zone, nil
}

func (c *cfClient) recordPayload(service, proto, srvName, recordName, target string, port, ttl, priority, weight int) ([]byte, error) {
	svc := service
	if !strings.HasPrefix(svc, "_") {
		svc = "_" + svc
	}
	prt := proto
	if !strings.HasPrefix(prt, "_") {
		prt = "_" + prt
	}
	rec := cfSRVRecord{
		Type: "SRV",
		Name: recordName,
		TTL:  ttl,
		Data: cfSRVData{
			Service:  svc,
			Proto:    prt,
			Name:     srvName,
			Priority: priority,
			Weight:   weight,
			Port:     port,
			Target:   target,
		},
	}
	return json.Marshal(rec)
}

func (c *cfClient) do(ctx context.Context, method, path string, body []byte) (json.RawMessage, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.apiURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var envelope cfResponse
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decode response (%d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !envelope.Success {
		msg := "cloudflare error"
		if len(envelope.Errors) > 0 {
			var errMsgs []string
			for _, e := range envelope.Errors {
				if e.Message != "" {
					errMsgs = append(errMsgs, fmt.Sprintf("%s (%d)", e.Message, e.Code))
				}
			}
			if len(errMsgs) > 0 {
				msg = strings.Join(errMsgs, ", ")
			}
		}
		return nil, fmt.Errorf("cloudflare API (%d): %s", resp.StatusCode, msg)
	}
	if envelope.Result == nil {
		return json.RawMessage("null"), nil
	}
	return *envelope.Result, nil
}
