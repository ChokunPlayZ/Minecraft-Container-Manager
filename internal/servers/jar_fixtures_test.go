package servers

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"fmt"
	"github.com/mcm-panel/mcm/internal/jars"
	"io"
	"net/http"
	"strings"
	"testing"
)

func fixtureJar(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	zw := zip.NewWriter(&buffer)
	entry, err := zw.Create("META-INF/MANIFEST.MF")
	if err != nil {
		t.Fatal(err)
	}
	entry.Write([]byte("Manifest-Version: 1.0\n"))
	if err = zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// All software fixtures remain in memory; lifecycle tests never fetch production artifacts.
func fixtureJarResolver(t *testing.T, resolver *jars.Resolver) *jars.Resolver {
	t.Helper()
	jar := fixtureJar(t)
	resolver.Client = &http.Client{Transport: fixtureRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.URL.Hostname() == "127.0.0.1" || req.URL.Hostname() == "localhost" || req.URL.Hostname() == "::1" {
			return http.DefaultTransport.RoundTrip(req)
		}
		path := req.URL.Path
		data := ""
		contentType := "application/json"
		switch {
		case strings.HasSuffix(path, ".jar") || strings.HasSuffix(path, "/server/jar"):
			data = string(jar)
			contentType = "application/java-archive"
		case strings.Contains(path, "/versions/loader/"):
			data = `[{"loader":{"version":"0.16.5"}},{"loader":{"version":"0.16.0"}},{"loader":{"version":"0.15.11"}}]`
		case strings.HasSuffix(path, "maven-metadata.json"):
			data = `{"1.20.1":["1.20.1-47.2.0"],"1.21.1":["1.21.1-52.0.14"]}`
		case strings.HasSuffix(path, "/builds"):
			data = `[{"id":120},{"id":133},{"id":145},{"id":232},{"id":436}]`
		case strings.Contains(path, "/builds/"):
			data = `{"downloads":{"server:default":{"url":"https://fixture.test/server.jar"}}}`
		case strings.HasSuffix(path, "/versions"):
			data = `{"versions":["1.21.1","1.21.4"]}`
		default:
			return nil, fmt.Errorf("unexpected software fixture request: %s", req.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(data)), ContentLength: int64(len(data)), Request: req}, nil
	})}
	return resolver
}

type fixtureRoundTrip func(*http.Request) (*http.Response, error)

func (f fixtureRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fixtureJarSHA1(t *testing.T) string { return fmt.Sprintf("%x", sha1.Sum(fixtureJar(t))) }
