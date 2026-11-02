package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()

	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return p
}

const validDomain = `"example.com": {
	"imap": {"host": "imap.example.com", "port": 993},
	"smtp": {"host": "smtp.example.com", "port": 465}
}`

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name: "valid",
			body: `{"listen": ":8080", "domains": {` + validDomain + `}}`,
		},
		{
			name: "valid pprof loopback",
			body: `{"pprof_listen": "127.0.0.1:6060", "domains": {` + validDomain + `}}`,
		},
		{
			name: "valid pprof localhost",
			body: `{"pprof_listen": "localhost:6060", "domains": {` + validDomain + `}}`,
		},
		{
			name:    "pprof public bind",
			body:    `{"pprof_listen": "0.0.0.0:6060", "domains": {` + validDomain + `}}`,
			wantErr: true,
		},
		{
			name:    "pprof without port",
			body:    `{"pprof_listen": "127.0.0.1", "domains": {` + validDomain + `}}`,
			wantErr: true,
		},
		{
			name:    "no domains",
			body:    `{"listen": ":8080"}`,
			wantErr: true,
		},
		{
			name:    "unknown field",
			body:    `{"listen": ":8080", "domains": {` + validDomain + `}, "extra": true}`,
			wantErr: true,
		},
		{
			name:    "missing imap",
			body:    `{"domains": {"example.com": {"smtp": {"host": "smtp.example.com", "port": 465}}}}`,
			wantErr: true,
		},
		{
			name:    "missing smtp",
			body:    `{"domains": {"example.com": {"imap": {"host": "imap.example.com", "port": 993}}}}`,
			wantErr: true,
		},
		{
			name:    "port zero",
			body:    `{"domains": {"example.com": {"imap": {"host": "imap.example.com", "port": 0}, "smtp": {"host": "smtp.example.com", "port": 465}}}}`,
			wantErr: true,
		},
		{
			name:    "port too high",
			body:    `{"domains": {"example.com": {"imap": {"host": "imap.example.com", "port": 65536}, "smtp": {"host": "smtp.example.com", "port": 465}}}}`,
			wantErr: true,
		},
		{
			name:    "invalid domain",
			body:    `{"domains": {"exa mple.com": {"imap": {"host": "imap.example.com", "port": 993}, "smtp": {"host": "smtp.example.com", "port": 465}}}}`,
			wantErr: true,
		},
		{
			name:    "invalid host",
			body:    `{"domains": {"example.com": {"imap": {"host": "imap.example.com/path", "port": 993}, "smtp": {"host": "smtp.example.com", "port": 465}}}}`,
			wantErr: true,
		},
		{
			name:    "duplicate domain",
			body:    `{"domains": {"Example.com": {"imap": {"host": "imap.example.com", "port": 993}, "smtp": {"host": "smtp.example.com", "port": 465}}, "example.com": {"imap": {"host": "imap.example.com", "port": 993}, "smtp": {"host": "smtp.example.com", "port": 465}}}}`,
			wantErr: true,
		},
		{
			name:    "trailing data",
			body:    `{"domains": {` + validDomain + `}}{}`,
			wantErr: true,
		},
		{
			name:    "not json",
			body:    "listen: :8080",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewConfig(writeConfig(t, tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewConfigDefaults(t *testing.T) {
	c, err := NewConfig(writeConfig(t, `{"domains": {`+validDomain+`}}`))
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}

	if c.Listen != ":8080" {
		t.Errorf("Listen = %q, want %q", c.Listen, ":8080")
	}
	if c.EnableApple {
		t.Error("EnableApple = true, want false")
	}
	if c.PprofListen != "" {
		t.Errorf("PprofListen = %q, want empty", c.PprofListen)
	}
	if _, ok := c.Domains["example.com"]; !ok {
		t.Errorf("domain example.com was not loaded: %v", c.Domains)
	}
}

func TestNewConfigLowercasesDomain(t *testing.T) {
	c, err := NewConfig(writeConfig(t, `{"domains": {"Example.COM": {"imap": {"host": "imap.example.com", "port": 993}, "smtp": {"host": "smtp.example.com", "port": 465}}}}`))
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}

	if _, ok := c.Domains["example.com"]; !ok {
		t.Errorf("domain was not lowercased: %v", c.Domains)
	}
}

func TestNewConfigServerOptions(t *testing.T) {
	c, err := NewConfig(writeConfig(t, `{"domains": {"example.com": {
		"display_name": "Example Mail",
		"imap": {"host": "imap.example.com", "port": 143, "socket_type": "starttls", "authentication": "OAuth2"},
		"smtp": {"host": "smtp.example.com", "port": 587, "socket_type": "PLAIN"}
	}}}`))
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}

	d := c.Domains["example.com"]
	if d.DisplayName != "Example Mail" {
		t.Errorf("DisplayName = %q", d.DisplayName)
	}
	if d.IMAP.SocketType != "STARTTLS" {
		t.Errorf("IMAP SocketType = %q, want STARTTLS", d.IMAP.SocketType)
	}
	if d.IMAP.Authentication != "OAuth2" {
		t.Errorf("IMAP Authentication = %q, want OAuth2", d.IMAP.Authentication)
	}
	if d.SMTP.SocketType != "plain" {
		t.Errorf("SMTP SocketType = %q, want plain", d.SMTP.SocketType)
	}
}

func TestNewConfigRejectsInvalidServerOptions(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "negative rate limit",
			body: `{"rate_limit_per_minute": -1, "domains": {` + validDomain + `}}`,
		},
		{
			name: "invalid socket type",
			body: `{"domains": {"example.com": {"imap": {"host": "imap.example.com", "port": 993, "socket_type": "tls"}, "smtp": {"host": "smtp.example.com", "port": 465}}}}`,
		},
		{
			name: "invalid authentication",
			body: `{"domains": {"example.com": {"imap": {"host": "imap.example.com", "port": 993, "authentication": "bogus"}, "smtp": {"host": "smtp.example.com", "port": 465}}}}`,
		},
		{
			name: "display name newline",
			body: `{"domains": {"example.com": {"display_name": "Example\nMail", "imap": {"host": "imap.example.com", "port": 993}, "smtp": {"host": "smtp.example.com", "port": 465}}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewConfig(writeConfig(t, tt.body)); err == nil {
				t.Fatal("NewConfig() error = nil, want error")
			}
		})
	}
}
