package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func FuzzHostname(f *testing.F) {
	for _, seed := range []string{
		"",
		"example.com",
		"autoconfig.Example.com:443",
		"[::1]:8080",
		"a b",
		strings.Repeat("a", 300),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, host string) {
		_ = hostname(host)
	})
}

func FuzzOutlookBody(f *testing.F) {
	for _, seed := range []string{
		"",
		`<Autodiscover><Request><EMailAddress>user@example.com</EMailAddress></Request></Autodiscover>`,
		`<Autodiscover><Request><EMailAddress>x&lt;/LoginName&gt;&lt;LoginName&gt;evil</EMailAddress></Request></Autodiscover>`,
		`<not-xml`,
	} {
		f.Add(seed)
	}

	router := NewRouter(testConfig())

	f.Fuzz(func(t *testing.T, body string) {
		if len(body) > maxBodyBytes+1024 {
			t.Skip()
		}

		r := httptest.NewRequest(http.MethodPost, "/autodiscover/autodiscover.xml", strings.NewReader(body))
		r.Host = "autodiscover.example.com"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, r)

		switch rec.Code {
		case http.StatusOK, http.StatusBadRequest, http.StatusRequestEntityTooLarge:
		default:
			t.Fatalf("unexpected status %d for body %q", rec.Code, body)
		}
	})
}

func FuzzRequestHost(f *testing.F) {
	for _, seed := range []string{
		"",
		"example.com",
		"autoconfig.example.com",
		"autoconfig.example.com:8080",
		"unknown.test",
		"a b",
	} {
		f.Add(seed)
	}

	router := NewRouter(testConfig())

	f.Fuzz(func(t *testing.T, host string) {
		r := httptest.NewRequest(http.MethodGet, "/mail/config-v1.1.xml", nil)
		r.Host = host
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, r)

		if rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
			t.Fatalf("unexpected status %d for host %q", rec.Code, host)
		}
	})
}
