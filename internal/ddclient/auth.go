package ddclient

import (
	"context"
	"fmt"
)

// authRequest is the provisional DD login body. Validate against a live appliance.
type authRequest struct {
	AuthInfo struct {
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"auth_info"`
}

// ensureToken logs in if no token is cached, capturing X-DD-AUTH-TOKEN.
func (c *SystemClient) ensureToken(ctx context.Context) error {
	if c.currentToken() != "" {
		return nil
	}
	var body authRequest
	body.AuthInfo.Username = c.cfg.Username
	body.AuthInfo.Password = c.cfg.Password

	resp, err := c.rc.R().SetContext(ctx).SetBody(body).Post("/api/v1/auth")
	if err != nil {
		return fmt.Errorf("auth POST: %w", err)
	}
	if resp.StatusCode() >= 300 {
		return fmt.Errorf("auth POST: status %d", resp.StatusCode())
	}
	tok := resp.Header().Get("X-DD-AUTH-TOKEN")
	if tok == "" {
		return fmt.Errorf("auth POST: no X-DD-AUTH-TOKEN in response")
	}
	c.mu.Lock()
	c.token = tok
	c.mu.Unlock()
	return nil
}

// Close logs out (best effort) and is safe to call with no active session.
func (c *SystemClient) Close() error {
	if c.currentToken() == "" {
		return nil
	}
	_, _ = c.rc.R().SetHeader("X-DD-AUTH-TOKEN", c.currentToken()).Delete("/api/v1/auth")
	c.clearToken()
	return nil
}
