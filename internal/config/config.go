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
	Name               string `yaml:"name"`
	Host               string `yaml:"host"`
	Port               int    `yaml:"port"` // defaults to 3009
	Username           string `yaml:"username"`
	Password           string `yaml:"password"`
	PasswordFile       string `yaml:"passwordFile"`
	InsecureSkipVerify bool   `yaml:"insecureSkipVerify"`
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

var envRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func interpolate(s string) string {
	return envRef.ReplaceAllStringFunc(s, func(m string) string {
		return os.Getenv(envRef.FindStringSubmatch(m)[1])
	})
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
		s.Password = interpolate(s.Password)
		if s.PasswordFile != "" && s.Password == "" {
			b, err := os.ReadFile(s.PasswordFile)
			if err != nil {
				return nil, fmt.Errorf("system %s passwordFile: %w", s.Name, err)
			}
			s.Password = strings.TrimSpace(string(b))
		}
	}
	if cfg.Server.Port == "" {
		cfg.Server.Port = "9099"
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
