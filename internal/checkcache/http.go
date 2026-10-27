package checkcache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
)

type observerKey struct{}
type observer struct {
	mu     sync.Mutex
	errors []error
}

// Observe catches upstream failures even when a catalog resolver falls back to
// a curated installation list. Fallbacks cannot prove an update is available.
func Observe(ctx context.Context) (context.Context, func() error) {
	o := &observer{}
	return context.WithValue(ctx, observerKey{}, o), func() error {
		o.mu.Lock()
		defer o.mu.Unlock()
		return errors.Join(o.errors...)
	}
}

func RecordError(ctx context.Context, err error) {
	if o, ok := ctx.Value(observerKey{}).(*observer); ok && err != nil {
		o.mu.Lock()
		o.errors = append(o.errors, err)
		o.mu.Unlock()
	}
}

type Transport struct{ Base http.RoundTripper }
type response struct {
	StatusCode int
	Status     string
	Header     http.Header
	Body       []byte
}

// RoundTrip caches raw GET responses, including non-JSON catalog endpoints.
func (t Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	if req.Method != http.MethodGet || !Active(req.Context()) {
		return base.RoundTrip(req)
	}
	headers := sha256.Sum256([]byte(fmt.Sprint(req.Header)))
	value, err := Do(req.Context(), fmt.Sprintf("http:%s:%x", req.URL.String(), headers), func() (*response, error) {
		res, err := base.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s returned status %d", req.URL.Hostname(), res.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(res.Body, 10<<20))
		if err != nil {
			return nil, err
		}
		return &response{StatusCode: res.StatusCode, Status: res.Status, Header: res.Header.Clone(), Body: body}, nil
	})
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: value.StatusCode, Status: value.Status, Header: value.Header.Clone(), Body: io.NopCloser(bytes.NewReader(value.Body)), ContentLength: int64(len(value.Body)), Request: req}, nil
}
