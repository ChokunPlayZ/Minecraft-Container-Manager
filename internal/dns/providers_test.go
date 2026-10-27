package dns

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"strings"
	"testing"
	"time"
)

func testRecord() *Record {
	return &Record{Name: "_minecraft._tcp.play.games.example.com", Subdomain: "play", Target: "node.example.com", Port: 25570, TTL: 600, Priority: 10, Weight: 0}
}

func TestPorkbunRecordLifecycle(t *testing.T) {
	ft := &fakeTransport{responses: []*http.Response{
		jsonResponse(200, `{"status":"SUCCESS","records":[]}`),
		jsonResponse(200, `{"status":"SUCCESS","id":12345}`),
		jsonResponse(200, `{"status":"SUCCESS"}`),
		jsonResponse(200, `{"status":"SUCCESS"}`),
		jsonResponse(200, `{"status":"SUCCESS","records":[]}`),
	}}
	p := &porkbunProvider{cfg: &config{Zone: "example.com", APIToken: "pk_test", APISecret: "sk_test"}, http: &http.Client{Transport: ft}}
	rec := testRecord()
	id, err := p.upsert(context.Background(), nil, rec)
	if err != nil || id != "12345" {
		t.Fatalf("create: id=%q err=%v", id, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(ft.requests[1].body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["name"] != "_minecraft._tcp.play.games" || payload["content"] != "0 25570 node.example.com" || payload["prio"] != float64(10) || payload["ttl"] != float64(600) || payload["apikey"] != "pk_test" || payload["secretapikey"] != "sk_test" {
		t.Fatalf("wrong Porkbun SRV payload: %#v", payload)
	}
	rec.RecordID = id
	if _, err := p.upsert(context.Background(), rec, rec); err != nil {
		t.Fatal(err)
	}
	if err := p.remove(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	if _, err := p.verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i, path := range []string{"/dns/retrieveByNameType/example.com/SRV/_minecraft._tcp.play.games", "/dns/create/example.com", "/dns/edit/example.com/12345", "/dns/delete/example.com/12345", "/dns/retrieve/example.com"} {
		if ft.requests[i].req.Method != http.MethodPost || !strings.HasSuffix(ft.requests[i].req.URL.Path, path) {
			t.Fatalf("request %d: %s", i, ft.requests[i].req.URL)
		}
	}
}

func TestDigitalOceanRecordLifecycle(t *testing.T) {
	ft := &fakeTransport{responses: []*http.Response{
		jsonResponse(200, `{"domain_records":[]}`),
		jsonResponse(201, `{"domain_record":{"id":12345}}`),
		jsonResponse(200, `{"domain_record":{"id":12345}}`),
		jsonResponse(204, ""),
		jsonResponse(200, `{"domain":{"name":"example.com"}}`),
	}}
	p := &digitalOceanProvider{cfg: &config{Zone: "example.com", APIToken: "do_test"}, http: &http.Client{Transport: ft}}
	rec := testRecord()
	id, err := p.upsert(context.Background(), nil, rec)
	if err != nil || id != "12345" {
		t.Fatalf("create: id=%q err=%v", id, err)
	}
	var payload doRecord
	if err := json.Unmarshal(ft.requests[1].body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Name != "_minecraft._tcp.play.games" || payload.Data != rec.Target || payload.Port != rec.Port || payload.Weight != 0 || payload.Priority != 10 {
		t.Fatalf("wrong DigitalOcean SRV payload: %#v", payload)
	}
	if query := ft.requests[0].req.URL.Query(); query.Get("type") != "SRV" || query.Get("name") != rec.Name {
		t.Fatalf("wrong lookup: %v", query)
	}
	rec.RecordID = id
	if _, err := p.upsert(context.Background(), rec, rec); err != nil {
		t.Fatal(err)
	}
	if err := p.remove(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	if _, err := p.verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i, method := range []string{"GET", "POST", "PUT", "DELETE", "GET"} {
		if ft.requests[i].req.Method != method || ft.requests[i].req.Header.Get("Authorization") != "Bearer do_test" {
			t.Fatalf("request %d: %#v", i, ft.requests[i].req)
		}
	}
}

func TestProvidersReuseExistingRecords(t *testing.T) {
	for _, provider := range []string{"cloudflare", "porkbun", "digitalocean"} {
		t.Run(provider, func(t *testing.T) {
			var responses []*http.Response
			switch provider {
			case "cloudflare":
				responses = []*http.Response{jsonResponse(200, `{"success":true,"result":[{"id":"12"}]}`), jsonResponse(200, `{"success":true,"result":{"id":"12"}}`)}
			case "porkbun":
				responses = []*http.Response{jsonResponse(200, `{"status":"SUCCESS","records":[{"id":"12"}]}`), jsonResponse(200, `{"status":"SUCCESS"}`)}
			case "digitalocean":
				responses = []*http.Response{jsonResponse(200, `{"domain_records":[{"id":12}]}`), jsonResponse(200, `{"domain_record":{"id":12}}`)}
			}
			ft := &fakeTransport{responses: responses}
			svc := &Service{http: &http.Client{Transport: ft}}
			p, err := svc.provider(&config{Provider: provider, Zone: "example.com"})
			if err != nil {
				t.Fatal(err)
			}
			id, err := p.upsert(context.Background(), nil, testRecord())
			if err != nil || id != "12" || len(ft.requests) != 2 {
				t.Fatalf("reuse: id=%q err=%v requests=%d", id, err, len(ft.requests))
			}
		})
	}
}

func TestProvidersKeepFailuresFromCreatingDuplicates(t *testing.T) {
	for _, provider := range []string{"cloudflare", "porkbun", "digitalocean"} {
		t.Run(provider, func(t *testing.T) {
			body := `{"message":"permission denied"}`
			if provider == "cloudflare" {
				body = `{"success":false,"errors":[{"message":"permission denied"}]}`
			}
			if provider == "porkbun" {
				body = `{"status":"ERROR","message":"permission denied"}`
			}
			ft := &fakeTransport{responses: []*http.Response{jsonResponse(403, body)}}
			svc := &Service{http: &http.Client{Transport: ft}}
			p, err := svc.provider(&config{Provider: provider, Zone: "example.com"})
			if err != nil {
				t.Fatal(err)
			}
			current := testRecord()
			current.RecordID = "12"
			if _, err := p.upsert(context.Background(), current, testRecord()); err == nil {
				t.Fatal("expected permission error")
			}
			if len(ft.requests) != 1 {
				t.Fatalf("retried a failed update: %d requests", len(ft.requests))
			}
		})
	}
}

func TestRoute53RenameAndRemove(t *testing.T) {
	old := `<ListResourceRecordSetsResponse><ResourceRecordSets><ResourceRecordSet><Name>_minecraft._tcp.old.example.com.</Name><Type>SRV</Type><TTL>120</TTL><ResourceRecords><ResourceRecord><Value>0 5 25565 node.example.com.</Value></ResourceRecord></ResourceRecords></ResourceRecordSet></ResourceRecordSets></ListResourceRecordSetsResponse>`
	ft := &fakeTransport{responses: []*http.Response{
		jsonResponse(200, old), jsonResponse(200, `<ChangeResourceRecordSetsResponse/>`),
		jsonResponse(200, strings.ReplaceAll(old, "old.example.com", "play.games.example.com")), jsonResponse(200, `<ChangeResourceRecordSetsResponse/>`),
	}}
	p := &route53Provider{cfg: &config{Zone: "Z123", APIToken: "AKIATEST", APISecret: "secret", SessionToken: "session"}, http: &http.Client{Transport: ft}}
	current := testRecord()
	current.Name = "_minecraft._tcp.old.example.com"
	desired := testRecord()
	if _, err := p.upsert(context.Background(), current, desired); err != nil {
		t.Fatal(err)
	}
	var batch struct {
		Changes []route53Change `xml:"ChangeBatch>Changes>Change"`
	}
	if err := xml.Unmarshal(ft.requests[1].body, &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Changes) != 2 || batch.Changes[0].Action != "DELETE" || batch.Changes[1].Action != "UPSERT" || batch.Changes[1].Record.Values[0] != "10 0 25570 node.example.com." {
		t.Fatalf("wrong rename batch: %#v", batch)
	}
	if err := p.remove(context.Background(), desired); err != nil {
		t.Fatal(err)
	}
	batch.Changes = nil
	if err := xml.Unmarshal(ft.requests[3].body, &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Changes) != 1 || batch.Changes[0].Action != "DELETE" || batch.Changes[0].Record.TTL != 120 {
		t.Fatalf("delete must use exact remote values: %#v", batch)
	}
	for _, req := range ft.requests {
		if req.req.Header.Get("X-Amz-Security-Token") != "session" || !strings.Contains(req.req.Header.Get("Authorization"), "/us-east-1/route53/aws4_request") {
			t.Fatalf("missing Route 53 authentication: %v", req.req.Header)
		}
	}
}

func TestRoute53SignatureIncludesQueryAndSessionToken(t *testing.T) {
	p := &route53Provider{cfg: &config{APIToken: "AKIATEST", APISecret: "secret", SessionToken: "session"}}
	req, _ := http.NewRequest(http.MethodGet, route53API+"/hostedzone/Z123/rrset?type=SRV&name=_minecraft._tcp.play.example.com&maxitems=1", nil)
	p.sign(req, nil, time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	first := req.Header.Get("Authorization")
	req.URL.RawQuery = "type=SRV&name=_minecraft._tcp.other.example.com&maxitems=1"
	p.sign(req, nil, time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	if first == req.Header.Get("Authorization") || !strings.Contains(first, "SignedHeaders=host;x-amz-date;x-amz-security-token") {
		t.Fatal("signature did not cover query and session token")
	}
}
