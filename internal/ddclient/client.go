// Package ddclient is the per-appliance Dell PowerProtect DD REST API client.
package ddclient

import "context"

// Client is the per-system DD API client abstraction. It is satisfied by the live
// SystemClient and by Mock (tests). Get authenticates lazily and decodes JSON.
type Client interface {
	// Name returns the configured system name (used as the `system` label).
	Name() string
	// Get fetches an absolute API path (e.g. "/rest/v1.0/system")
	// and JSON-decodes the body into out. It (re-)authenticates as needed.
	Get(ctx context.Context, path string, out any) error
	// Close releases the session (DELETE /rest/v1.0/auth) and HTTP resources.
	Close() error
}
