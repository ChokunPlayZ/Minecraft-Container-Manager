package dns

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const route53API = "https://route53.amazonaws.com/2013-04-01"

type route53Provider struct {
	cfg  *config
	http *http.Client
}

type route53Record struct {
	Name          string   `xml:"Name"`
	Type          string   `xml:"Type"`
	SetIdentifier string   `xml:"SetIdentifier,omitempty"`
	TTL           int      `xml:"TTL"`
	Values        []string `xml:"ResourceRecords>ResourceRecord>Value"`
}

type route53Change struct {
	Action string        `xml:"Action"`
	Record route53Record `xml:"ResourceRecordSet"`
}

func (p *route53Provider) path() string {
	return "/hostedzone/" + url.PathEscape(strings.TrimPrefix(p.cfg.Zone, "/hostedzone/"))
}

func (p *route53Provider) upsert(ctx context.Context, current, desired *Record) (string, error) {
	var changes []route53Change
	if current != nil && current.Name != desired.Name {
		old, err := p.find(ctx, current.Name)
		if err != nil {
			return "", err
		}
		if old != nil {
			changes = append(changes, route53Change{Action: "DELETE", Record: *old})
		}
	}
	changes = append(changes, route53Change{Action: "UPSERT", Record: route53Record{
		Name: desired.Name, Type: "SRV", TTL: desired.TTL,
		Values: []string{fmt.Sprintf("%d %d %d %s", desired.Priority, desired.Weight, desired.Port, strings.TrimSuffix(desired.Target, ".")+".")},
	}})
	return desired.Name, p.change(ctx, changes)
}

func (p *route53Provider) remove(ctx context.Context, rec *Record) error {
	// DELETE must contain the exact current TTL and values, so read them first.
	current, err := p.find(ctx, rec.Name)
	if err != nil {
		return err
	}
	if current == nil {
		return nil
	}
	return p.change(ctx, []route53Change{{Action: "DELETE", Record: *current}})
}

func (p *route53Provider) find(ctx context.Context, name string) (*route53Record, error) {
	query := url.Values{"name": {name}, "type": {"SRV"}, "maxitems": {"1"}}
	var result struct {
		Records []route53Record `xml:"ResourceRecordSets>ResourceRecordSet"`
	}
	if err := p.do(ctx, http.MethodGet, p.path()+"/rrset?"+query.Encode(), nil, &result); err != nil {
		return nil, err
	}
	if len(result.Records) == 0 {
		return nil, nil
	}
	rec := &result.Records[0]
	if rec.Type != "SRV" || !strings.EqualFold(strings.TrimSuffix(rec.Name, "."), strings.TrimSuffix(name, ".")) {
		return nil, nil
	}
	if rec.SetIdentifier != "" {
		return nil, fmt.Errorf("Route 53 record %q uses a routing policy; only simple SRV records are supported", name)
	}
	return rec, nil
}

func (p *route53Provider) change(ctx context.Context, changes []route53Change) error {
	body, err := xml.Marshal(struct {
		XMLName xml.Name        `xml:"ChangeResourceRecordSetsRequest"`
		XMLNS   string          `xml:"xmlns,attr"`
		Changes []route53Change `xml:"ChangeBatch>Changes>Change"`
	}{XMLNS: "https://route53.amazonaws.com/doc/2013-04-01/", Changes: changes})
	if err != nil {
		return err
	}
	return p.do(ctx, http.MethodPost, p.path()+"/rrset", body, nil)
}

func (p *route53Provider) verify(ctx context.Context) (*zoneInfo, error) {
	var result struct {
		Name    string `xml:"HostedZone>Name"`
		Private bool   `xml:"HostedZone>Config>PrivateZone"`
	}
	if err := p.do(ctx, http.MethodGet, p.path(), nil, &result); err != nil {
		return nil, err
	}
	if result.Private {
		return nil, fmt.Errorf("Route 53 hosted zone is private; select a public hosted zone for player addresses")
	}
	return &zoneInfo{Name: strings.TrimSuffix(result.Name, "."), Status: "active"}, nil
}

func (p *route53Provider) do(ctx context.Context, method, path string, body []byte, result any) error {
	req, err := http.NewRequestWithContext(ctx, method, route53API+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/xml")
	p.sign(req, body, time.Now().UTC())
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
			Message string `xml:"Error>Message"`
		}
		_ = xml.Unmarshal(raw, &failure)
		return &providerError{resp.StatusCode, fmt.Sprintf("Route 53 API (%d): %s", resp.StatusCode, failure.Message)}
	}
	if result == nil {
		return nil
	}
	return xml.Unmarshal(raw, result)
}

// Route 53's global endpoint signs with the us-east-1 region. Include query
// parameters and the STS session token in the signature when present.
func (p *route53Provider) sign(req *http.Request, body []byte, now time.Time) {
	hash := func(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
	mac := func(key []byte, data string) []byte {
		h := hmac.New(sha256.New, key)
		h.Write([]byte(data))
		return h.Sum(nil)
	}
	date := now.Format("20060102T150405Z")
	req.Header.Set("X-Amz-Date", date)
	headers := "host:" + req.URL.Host + "\n" + "x-amz-date:" + date + "\n"
	signed := "host;x-amz-date"
	if p.cfg.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", p.cfg.SessionToken)
		headers += "x-amz-security-token:" + p.cfg.SessionToken + "\n"
		signed += ";x-amz-security-token"
	}
	query := strings.ReplaceAll(req.URL.Query().Encode(), "+", "%20")
	canonical := strings.Join([]string{req.Method, req.URL.EscapedPath(), query, headers, signed, hash(body)}, "\n")
	scope := now.Format("20060102") + "/us-east-1/route53/aws4_request"
	toSign := strings.Join([]string{"AWS4-HMAC-SHA256", date, scope, hash([]byte(canonical))}, "\n")
	key := mac([]byte("AWS4"+p.cfg.APISecret), now.Format("20060102"))
	key = mac(key, "us-east-1")
	key = mac(key, "route53")
	key = mac(key, "aws4_request")
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", p.cfg.APIToken, scope, signed, hex.EncodeToString(mac(key, toSign))))
}
