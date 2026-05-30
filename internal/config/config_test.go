package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadInterpolatesEnvAndDefaults(t *testing.T) {
	t.Setenv("DD01_PASSWORD", "s3cret")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
server: {host: "0.0.0.0", port: "9099", uri: "/metrics"}
collection: {interval: "5m", timeout: "60s"}
systems:
  - {name: dd01, host: dd01.example.com, username: u, password: "${DD01_PASSWORD}", insecureSkipVerify: true}
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Systems[0].Password != "s3cret" {
		t.Fatalf("password = %q, want interpolated s3cret", cfg.Systems[0].Password)
	}
	if cfg.Collection.Interval.String() != "5m0s" {
		t.Fatalf("interval = %s, want 5m0s", cfg.Collection.Interval)
	}
}

func TestLoadRejectsEmptySystems(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	_ = os.WriteFile(path, []byte("systems: []\n"), 0o600)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error when no systems configured")
	}
}
