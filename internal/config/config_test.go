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

func TestLoadFailsOnMissingEnvRef(t *testing.T) {
	// DD01_PASSWORD is intentionally unset: an unresolved ${VAR} must be a load
	// error, not a silent empty password that fails auth later.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
systems:
  - {name: dd01, host: dd01.example.com, username: u, password: "${DD01_PASSWORD}"}
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error when a referenced env var is unset")
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

func TestLoadTrimsPasswordFile(t *testing.T) {
	dir := t.TempDir()
	// Password file with trailing newline (as written by echo)
	pwFile := filepath.Join(dir, "password.txt")
	if err := os.WriteFile(pwFile, []byte("s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "config.yaml")
	yaml := `
systems:
  - {name: dd01, host: dd01.example.com, username: u, passwordFile: ` + pwFile + `}
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Systems[0].Password != "s3cret" {
		t.Fatalf("password = %q, want %q (no trailing newline)", cfg.Systems[0].Password, "s3cret")
	}
}
