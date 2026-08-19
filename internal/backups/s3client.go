package backups

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// s3Client is a minimal path-style S3-compatible HTTP client with AWS SigV4
// signing. It keeps the package dependency-free so it builds and runs against
// MinIO, AWS S3, and other S3-compatible stores without requiring the SDK.
type s3Client struct {
	endpoint   string
	bucket     string
	region     string
	accessKey  string
	secretKey  string
	httpClient *http.Client
}

func newS3Client(cfg S3Config) *s3Client {
	return &s3Client{
		endpoint:   strings.TrimSuffix(cfg.Endpoint, "/"),
		bucket:     cfg.Bucket,
		region:     cfg.Region,
		accessKey:  cfg.AccessKey,
		secretKey:  cfg.SecretKey,
		httpClient: &http.Client{Timeout: 0},
	}
}

// objectURL returns the path-style URL for a key inside the configured bucket.
// Keys are generated internally from UUIDs, so segments are URL safe.
func (c *s3Client) objectURL(key string) string {
	u, _ := url.Parse(c.endpoint)
	u.Path = strings.TrimSuffix(u.Path, "/") + "/" + c.bucket + "/" + key
	return u.String()
}

// putObject uploads body to key. It chunks the reader when the store supports
// streaming writes by reading the whole payload into memory for simplicity.
func (c *s3Client) putObject(ctx context.Context, key string, body io.Reader) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("read upload body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.objectURL(key), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	c.sign(req, sha256Bytes(data))
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload object: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("upload object %s: unexpected status %s", key, resp.Status)
	}
	return nil
}

// getObject streams the object at key from the store.
func (c *s3Client) getObject(ctx context.Context, key string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.objectURL(key), nil)
	if err != nil {
		return nil, err
	}
	c.sign(req, sha256Bytes(nil))
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("get object %s: unexpected status %s", key, resp.Status)
	}
	return resp.Body, nil
}

// deleteObject removes the object at key. A 404 is treated as success.
func (c *s3Client) deleteObject(ctx context.Context, key string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.objectURL(key), nil)
	if err != nil {
		return err
	}
	c.sign(req, sha256Bytes(nil))
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("delete object %s: unexpected status %s", key, resp.Status)
	}
	return nil
}

// sign applies AWS Signature Version 4 to the request.
func (c *s3Client) sign(req *http.Request, payloadHash []byte) {
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	payloadHashHex := hex.EncodeToString(payloadHash)

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHashHex)

	// Canonical request components. Query strings are ignored for the simple
	// operations we perform (single objects, fixed headers).
	canonicalHeaders := "host:" + req.URL.Host + "\n" + "x-amz-content-sha256:" + payloadHashHex + "\n" + "x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		"",
		canonicalHeaders,
		signedHeaders,
		payloadHashHex,
	}, "\n")

	scope := dateStamp + "/" + c.region + "/s3/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		sha256Hex(canonicalRequest),
	}, "\n")

	kDate := hmacSHA256([]byte("AWS4"+c.secretKey), dateStamp)
	kRegion := hmacSHA256(kDate, c.region)
	kService := hmacSHA256(kRegion, "s3")
	kSigning := hmacSHA256(kService, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))

	auth := fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		c.accessKey, scope, signedHeaders, signature,
	)
	req.Header.Set("Authorization", auth)
}

func sha256Bytes(b []byte) []byte {
	h := sha256.New()
	h.Write(b)
	return h.Sum(nil)
}

func sha256Hex(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}
