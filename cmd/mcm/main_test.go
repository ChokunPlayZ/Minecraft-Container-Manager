package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPSRedirect(t *testing.T) {
	for _, tc := range []struct{ addr, request, want string }{
		{":8080", "http://panel.example.com/servers/one?tab=console", "https://panel.example.com:8080/servers/one?tab=console"},
		{":8443", "http://panel.example.com:8081/login", "https://panel.example.com:8443/login"},
		{"[::]:8443", "http://[::1]:8081/login", "https://[::1]:8443/login"},
	} {
		t.Run(tc.request, func(t *testing.T) {
			w := httptest.NewRecorder()
			httpsRedirect(tc.addr).ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.request, nil))
			if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != tc.want {
				t.Fatalf("redirect = %d %q, want 301 %q", w.Code, w.Header().Get("Location"), tc.want)
			}
		})
	}
}
