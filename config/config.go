package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

type Config struct {
	Listen      string            `json:"listen"`
	PprofListen string            `json:"pprof_listen"`
	RateLimit   int               `json:"rate_limit_per_minute"`
	TrustProxy  bool              `json:"trust_proxy"`
	EnableApple bool              `json:"enable_apple"`
	Domains     map[string]Domain `json:"domains"`
}

type Domain struct {
	DisplayName string `json:"display_name"`
	IMAP        Server `json:"imap"`
	SMTP        Server `json:"smtp"`
}

type Server struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	STARTTLS       bool   `json:"starttls"`
	SocketType     string `json:"socket_type"`
	Authentication string `json:"authentication"`
}

func NewConfig(p string) (*Config, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}

	var c Config
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("config: unexpected data after JSON object")
	}

	if err := c.normalize(); err != nil {
		return nil, err
	}

	return &c, nil
}

func (c *Config) Validate() error {
	return c.normalize()
}

func (c *Config) normalize() error {
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	if strings.TrimSpace(c.Listen) != c.Listen || strings.ContainsAny(c.Listen, " \t\r\n") {
		return fmt.Errorf("config: listen address %q contains whitespace", c.Listen)
	}

	if c.PprofListen != "" {
		if err := validatePprofListen(c.PprofListen); err != nil {
			return err
		}
	}

	if c.RateLimit < 0 {
		return fmt.Errorf("config: rate_limit_per_minute must be >= 0")
	}

	if len(c.Domains) == 0 {
		return fmt.Errorf("config: at least one domain is required")
	}

	domains := make(map[string]Domain, len(c.Domains))
	for name, d := range c.Domains {
		key := strings.ToLower(strings.TrimSpace(name))
		if !validDomainName(key) {
			return fmt.Errorf("config: invalid domain %q", name)
		}
		if _, ok := domains[key]; ok {
			return fmt.Errorf("config: duplicate domain %q", key)
		}
		d.DisplayName = strings.TrimSpace(d.DisplayName)
		if strings.ContainsAny(d.DisplayName, "\r\n\t") {
			return fmt.Errorf("config: domain %q: display_name contains control characters", key)
		}
		if err := validateServer("imap", &d.IMAP); err != nil {
			return fmt.Errorf("config: domain %q: %w", key, err)
		}
		if err := validateServer("smtp", &d.SMTP); err != nil {
			return fmt.Errorf("config: domain %q: %w", key, err)
		}
		domains[key] = d
	}
	c.Domains = domains

	return nil
}

func validatePprofListen(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("config: pprof_listen %q: %w", addr, err)
	}

	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("config: pprof_listen must bind to loopback, got %q", addr)
	}

	return nil
}

func validDomainName(name string) bool {
	if name == "" || len(name) > 253 || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
		return false
	}

	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-', r == '.':
		default:
			return false
		}
	}

	return true
}

var socketTypes = map[string]string{
	"ssl":      "SSL",
	"starttls": "STARTTLS",
	"plain":    "plain",
}

var authentications = map[string]bool{
	"password-cleartext": true,
	"password-encrypted": true,
	"NTLM":               true,
	"GSSAPI":             true,
	"client-IP-address":  true,
	"TLS-client-cert":    true,
	"OAuth2":             true,
	"none":               true,
}

func validateServer(kind string, s *Server) error {
	if s.Host == "" || strings.TrimSpace(s.Host) != s.Host || strings.ContainsAny(s.Host, " \t\r\n/:") {
		return fmt.Errorf("%s: invalid host %q", kind, s.Host)
	}
	if s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("%s: port %d out of range 1..65535", kind, s.Port)
	}

	if s.SocketType != "" {
		v, ok := socketTypes[strings.ToLower(s.SocketType)]
		if !ok {
			return fmt.Errorf("%s: invalid socket_type %q", kind, s.SocketType)
		}
		s.SocketType = v
	}
	if s.Authentication != "" && !authentications[s.Authentication] {
		return fmt.Errorf("%s: invalid authentication %q", kind, s.Authentication)
	}

	return nil
}
