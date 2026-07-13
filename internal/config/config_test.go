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
server: {host: "0.0.0.0", port: "9441", uri: "/metrics"}
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

func TestLoadInterpolatesHostAndUsername(t *testing.T) {
	t.Setenv("DD01_HOSTNAME", "dd01.example.com")
	t.Setenv("DD01_USERNAME", "monitor")
	t.Setenv("DD01_PASSWORD", "s3cret")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
systems:
  - {name: dd01, host: "${DD01_HOSTNAME}", username: "${DD01_USERNAME}", password: "${DD01_PASSWORD}"}
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Systems[0].Host != "dd01.example.com" {
		t.Fatalf("host = %q, want dd01.example.com", cfg.Systems[0].Host)
	}
	if cfg.Systems[0].Username != "monitor" {
		t.Fatalf("username = %q, want monitor", cfg.Systems[0].Username)
	}
}

func TestLoadInterpolatesName(t *testing.T) {
	// A name like ${PPDD1_HOSTNAME} should resolve to a real label value, not be
	// carried through literally as the `system` metric label.
	t.Setenv("DD01_NAME", "dd01.example.com")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
systems:
  - {name: "${DD01_NAME}", host: dd01.example.com, username: u, password: "secret"}
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Systems[0].Name != "dd01.example.com" {
		t.Fatalf("name = %q, want dd01.example.com", cfg.Systems[0].Name)
	}
}

func TestLoadFailsOnMissingNameEnvRef(t *testing.T) {
	// DD01_NAME intentionally unset: an unresolved ${VAR} in name must be a load
	// error, not a silent literal ${VAR} label.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
systems:
  - {name: "${DD01_NAME}", host: dd01.example.com, username: u, password: "secret"}
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error when name env var is unset")
	}
}

func TestLoadFailsOnMissingHostEnvRef(t *testing.T) {
	// DD01_HOSTNAME intentionally unset: an unresolved ${VAR} in host must be a
	// load error, not a silent empty hostname that fails connection later.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
systems:
  - {name: dd01, host: "${DD01_HOSTNAME}", username: u, password: "secret"}
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error when host env var is unset")
	}
}

func TestLoadFailsOnMissingUsernameEnvRef(t *testing.T) {
	// DD01_USERNAME intentionally unset: an unresolved ${VAR} in username must be a
	// load error, not a silent empty username that fails auth later.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
systems:
  - {name: dd01, host: dd01.example.com, username: "${DD01_USERNAME}", password: "secret"}
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error when username env var is unset")
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

func TestLoadInsecureSkipVerifyNativeBool(t *testing.T) {
	// Existing native-bool configs must keep working after the EnvBool retype.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
systems:
  - {name: dd01, host: dd01.example.com, username: u, password: "secret", insecureSkipVerify: true}
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Systems[0].InsecureSkipVerify.Bool() {
		t.Fatal("insecureSkipVerify = false, want true (native bool)")
	}
}

func TestLoadInsecureSkipVerifyDefaultsFalse(t *testing.T) {
	// Omitted field must default to false, matching the pre-EnvBool zero value.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
systems:
  - {name: dd01, host: dd01.example.com, username: u, password: "secret"}
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Systems[0].InsecureSkipVerify.Bool() {
		t.Fatal("insecureSkipVerify = true, want false (default)")
	}
}

func TestLoadInsecureSkipVerifyEnvRefTrue(t *testing.T) {
	t.Setenv("PPDD1_SKIP_CERTIFICATE", "true")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
systems:
  - {name: dd01, host: dd01.example.com, username: u, password: "secret", insecureSkipVerify: "${PPDD1_SKIP_CERTIFICATE}"}
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Systems[0].InsecureSkipVerify.Bool() {
		t.Fatal("insecureSkipVerify = false, want true (env ref)")
	}
}

func TestLoadInsecureSkipVerifyEnvRefEmptyResolvesFalse(t *testing.T) {
	// Set but empty: interpolate() succeeds (var is present), and Resolve treats
	// an empty expansion as false rather than erroring.
	t.Setenv("PPDD1_SKIP_CERTIFICATE", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
systems:
  - {name: dd01, host: dd01.example.com, username: u, password: "secret", insecureSkipVerify: "${PPDD1_SKIP_CERTIFICATE}"}
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Systems[0].InsecureSkipVerify.Bool() {
		t.Fatal("insecureSkipVerify = true, want false (empty expansion)")
	}
}

func TestLoadInsecureSkipVerifyEnvRefUnsetIsLoadError(t *testing.T) {
	// Unset (not just empty) ${VAR} must fail fast, matching host/username/password policy.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
systems:
  - {name: dd01, host: dd01.example.com, username: u, password: "secret", insecureSkipVerify: "${PPDD1_SKIP_CERTIFICATE}"}
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error when insecureSkipVerify env var is unset")
	}
}

func TestLoadInsecureSkipVerifyNonBooleanErrors(t *testing.T) {
	t.Setenv("PPDD1_SKIP_CERTIFICATE", "maybe")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
systems:
  - {name: dd01, host: dd01.example.com, username: u, password: "secret", insecureSkipVerify: "${PPDD1_SKIP_CERTIFICATE}"}
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error when insecureSkipVerify env var is not a boolean")
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
