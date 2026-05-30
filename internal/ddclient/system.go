package ddclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"

	"github.com/go-resty/resty/v2"
)

// Config configures a SystemClient. HTTPClient is optional (tests inject the
// httptest TLS client); when nil a client honoring InsecureSkipVerify is built.
type Config struct {
	Name               string
	BaseURL            string // e.g. https://dd01.example.com:3009
	Username           string
	Password           string
	InsecureSkipVerify bool
	HTTPClient         *http.Client
}

// SystemClient is the live per-appliance DD REST client.
type SystemClient struct {
	cfg   Config
	rc    *resty.Client
	mu    sync.Mutex
	token string
}

// NewSystemClient builds a client. Auth is lazy (on first Get).
func NewSystemClient(cfg Config) *SystemClient {
	rc := resty.New().SetBaseURL(cfg.BaseURL)
	if cfg.HTTPClient != nil {
		rc.SetTransport(cfg.HTTPClient.Transport)
	} else if cfg.InsecureSkipVerify {
		rc.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}) //nolint:gosec // operator opt-in
	}
	// Retry on transport errors and 5xx, but never on 4xx (do not retry
	// auth/permission failures). resty passes r == nil on transport/TLS errors,
	// so guard the dereference to avoid a panic.
	rc.SetRetryCount(2).AddRetryCondition(func(r *resty.Response, err error) bool {
		if err != nil {
			return true
		}
		return r != nil && r.StatusCode() >= 500
	})
	return &SystemClient{cfg: cfg, rc: rc}
}

func (c *SystemClient) Name() string { return c.cfg.Name }

// Get fetches path, authenticating first if needed and re-authenticating once on 401.
func (c *SystemClient) Get(ctx context.Context, path string, out any) error {
	if err := c.ensureToken(ctx); err != nil {
		return err
	}
	resp, err := c.do(ctx, path, out)
	if err != nil {
		return err
	}
	if resp.StatusCode() == http.StatusUnauthorized {
		c.clearToken()
		if err := c.ensureToken(ctx); err != nil {
			return err
		}
		resp, err = c.do(ctx, path, out)
		if err != nil {
			return err
		}
	}
	if resp.StatusCode() >= 300 {
		return fmt.Errorf("GET %s: status %d", path, resp.StatusCode())
	}
	return nil
}

func (c *SystemClient) do(ctx context.Context, path string, out any) (*resty.Response, error) {
	return c.rc.R().SetContext(ctx).
		SetHeader("X-DD-AUTH-TOKEN", c.currentToken()).
		SetResult(out).
		Get(path)
}

func (c *SystemClient) currentToken() string { c.mu.Lock(); defer c.mu.Unlock(); return c.token }
func (c *SystemClient) clearToken()          { c.mu.Lock(); c.token = ""; c.mu.Unlock() }
