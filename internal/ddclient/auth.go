package ddclient

import (
	"context"
	"fmt"
)

// authPath is the documented DD login/logout endpoint (7.3 REST API guide).
const authPath = "/rest/v1.0/auth"

// authRequest is the DD login body. Per the 7.3 guide this is a FLAT
// {username,password} object posted to /rest/v1.0/auth.
//
// HIGHEST-RISK MAPPING: if logins fail against a live appliance, revert HERE
// first. The prior guess was {"auth_info":{...}} posted to /api/v1/auth.
type authRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ensureToken logs in if no token is cached, capturing X-DD-AUTH-TOKEN.
func (c *SystemClient) ensureToken(ctx context.Context) error {
	if c.currentToken() != "" {
		return nil
	}
	body := authRequest{Username: c.cfg.Username, Password: c.cfg.Password}

	resp, err := c.rc.R().SetContext(ctx).SetBody(body).Post(authPath)
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
	_, _ = c.rc.R().SetHeader("X-DD-AUTH-TOKEN", c.currentToken()).Delete(authPath)
	c.clearToken()
	return nil
}
