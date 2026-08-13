// Package config loads and validates the exporter configuration.
package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

// System is one DD appliance to monitor.
type System struct {
	Name               string  `yaml:"name"`
	Host               string  `yaml:"host"`
	Port               int     `yaml:"port"` // defaults to 3009
	Username           string  `yaml:"username"`
	Password           string  `yaml:"password"`
	PasswordFile       string  `yaml:"passwordFile"`
	InsecureSkipVerify EnvBool `yaml:"insecureSkipVerify"`
}

// BaseURL returns the https://host:port root for the DD REST API.
func (s System) BaseURL() string {
	port := s.Port
	if port == 0 {
		port = 3009
	}
	return fmt.Sprintf("https://%s:%d", s.Host, port)
}

// Server holds HTTP-server settings.
type Server struct {
	Host    string `yaml:"host"`
	Port    string `yaml:"port"`
	URI     string `yaml:"uri"`
	LogName string `yaml:"logName"`
}

// Collection holds loop timing.
type Collection struct {
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
}

// Config is the whole file.
type Config struct {
	Server     Server     `yaml:"server"`
	Collection Collection `yaml:"collection"`
	Systems    []System   `yaml:"systems"`
}

var envRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-[^}]*)?\}`)

// interpolate replaces every ${VAR} in s with its environment value, returning an
// error if any referenced variable is unset. Failing fast turns a typo'd secret
// name into a config-load error instead of repeated runtime auth failures.
//
// A reference may carry a fallback as ${VAR:-default}, borrowing the shell /
// docker-compose syntax and its meaning: unset OR empty falls back, and the reference
// never errors. That lets a shipped config.yaml drive a non-secret setting from the
// environment while still starting on a host that never exported it. Use it only where a
// safe default exists.
//
// A bare ${VAR} fails when the variable is UNSET; an exported-but-empty one expands to
// the empty string, as it always has. Credential fields get the stricter treatment —
// see interpolateSecret.
func interpolate(s string) (string, error) {
	var missing []string
	out := envRef.ReplaceAllStringFunc(s, func(m string) string {
		sub := envRef.FindStringSubmatch(m)
		name, fallback := sub[1], sub[2]
		v, ok := os.LookupEnv(name)
		if ok && v != "" {
			return v
		}
		if fallback != "" {
			return fallback[len(":-"):] // group 2 keeps its ":-" prefix, so "" means absent
		}
		if !ok {
			missing = append(missing, name)
		}
		return ""
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("unset environment variable(s): %s", strings.Join(missing, ", "))
	}
	return out, nil
}

// interpolateSecret expands like interpolate, but additionally rejects a credential that was
// written as an env reference yet resolves to nothing. A stray `PPDD1_PASSWORD=` line in
// a .env file is a plausible typo, and without this the exporter would authenticate with an
// empty credential and report a failure that names the wrong cause.
//
// It fires only when the field actually contains a ${...} reference: a literal value is
// passed through untouched and an omitted optional credential stays omitted, so it cannot
// break a config that never referenced the environment in the first place.
func interpolateSecret(field, s string) (string, error) {
	out, err := interpolate(s)
	if err != nil {
		return "", err
	}
	// Only the variable NAMES go into the error. The raw field value may itself contain
	// part of a credential (a mixed literal like "pw${VAR}"), and this error is logged.
	if out == "" {
		var names []string
		for _, m := range envRef.FindAllStringSubmatch(s, -1) {
			names = append(names, "${"+m[1]+"}")
		}
		if len(names) > 0 {
			return "", fmt.Errorf("%s references %s, which resolved to an empty value",
				field, strings.Join(names, ", "))
		}
	}
	return out, nil
}

// Load reads, interpolates ${ENV} references, applies defaults, and validates.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	for i := range cfg.Systems {
		s := &cfg.Systems[i]
		// Interpolate name first so a name like ${PPDD1_HOSTNAME} resolves to a real
		// label value and any later host/username errors quote the resolved name.
		name, err := interpolate(s.Name)
		if err != nil {
			return nil, fmt.Errorf("system %s name: %w", s.Name, err)
		}
		s.Name = name
		host, err := interpolateSecret("host", s.Host)
		if err != nil {
			return nil, fmt.Errorf("system %s host: %w", s.Name, err)
		}
		s.Host = host
		username, err := interpolateSecret("username", s.Username)
		if err != nil {
			return nil, fmt.Errorf("system %s username: %w", s.Name, err)
		}
		s.Username = username
		pw, err := interpolateSecret("password", s.Password)
		if err != nil {
			return nil, fmt.Errorf("system %s password: %w", s.Name, err)
		}
		s.Password = pw
		if s.PasswordFile != "" && s.Password == "" {
			b, err := os.ReadFile(s.PasswordFile)
			if err != nil {
				return nil, fmt.Errorf("system %s passwordFile: %w", s.Name, err)
			}
			s.Password = strings.TrimSpace(string(b))
		}
		if err := s.InsecureSkipVerify.Resolve(interpolate); err != nil {
			return nil, fmt.Errorf("system %s insecureSkipVerify: %w", s.Name, err)
		}
	}
	if cfg.Server.Port == "" {
		cfg.Server.Port = "9441"
	}
	if cfg.Server.URI == "" {
		cfg.Server.URI = "/metrics"
	}
	if cfg.Collection.Interval == 0 {
		cfg.Collection.Interval = 5 * time.Minute
	}
	if cfg.Collection.Timeout == 0 {
		cfg.Collection.Timeout = 60 * time.Second
	}
	if len(cfg.Systems) == 0 {
		return nil, fmt.Errorf("no systems configured")
	}
	return &cfg, nil
}
