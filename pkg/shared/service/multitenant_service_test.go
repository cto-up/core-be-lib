package service

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestExtractDomainParts(t *testing.T) {
	tests := []struct {
		hostname  string
		subdomain string
		domain    string
		tld       string
	}{
		{"example.com", "", "example", "com"},
		{"sub.example.com", "sub", "example", "com"},
		{"bo-corpa.cto.com", "bo-corpa", "cto", "com"},
		{"example.co.uk", "", "example", "co.uk"},
		{"sub.example.co.uk", "sub", "example", "co.uk"},
		{"bo-corpa.cto.co.uk", "bo-corpa", "cto", "co.uk"},
		{"localhost", "", "localhost", ""},
		{"192.168.1.1", "", "192.168.1.1", ""},
	}

	for _, test := range tests {
		t.Run(test.hostname, func(t *testing.T) {
			subdomain, domain, tld := extractDomainParts(test.hostname)
			if subdomain != test.subdomain {
				t.Errorf("Expected subdomain %q, got %q", test.subdomain, subdomain)
			}
			if domain != test.domain {
				t.Errorf("Expected domain %q, got %q", test.domain, domain)
			}
			if tld != test.tld {
				t.Errorf("Expected TLD %q, got %q", test.tld, tld)
			}
		})
	}
}

func TestGetSubdomain(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected string
	}{
		{"simple domain", "example.com", ""},
		{"standard subdomain", "sub.example.com", "sub"},
		{"specific case", "bo-corpa.cto.com", "corpa"},
		{"deep subdomain", "deep.bo-corpa.cto.com", "corpa"},
		{"country tld", "sub.example.co.uk", "sub"},
		{"country tld specific case", "bo-corpa.cto.co.uk", "corpa"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create a mock gin context with the test host
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "http://"+test.host, nil)
			c.Request.Header.Set("Host", test.host)

			subdomain, err := GetSubdomain(c)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if subdomain != test.expected {
				t.Errorf("Expected subdomain %q, got %q", test.expected, subdomain)
			}
		})
	}
}

func TestGetDomain(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected string
	}{
		{"simple domain", "example.com", "example.com"},
		{"standard subdomain", "sub.example.com", "example.com"},
		{"specific case", "bo.corpa.cto.com", "cto.com"},
		{"deep subdomain", "deep.bo.corpa.cto.com", "cto.com"},
		{"country tld", "example.co.uk", "example.co.uk"},
		{"country tld with subdomain", "sub.example.co.uk", "example.co.uk"},
		{"country tld specific case", "bo.corpa.cto.co.uk", "cto.co.uk"},
		{"localhost", "localhost", "localhost"},
		{"ip address", "192.168.1.1", "192.168.1.1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create a mock gin context with the test host
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "http://"+test.host, nil)
			c.Request.Header.Set("Host", test.host)

			domain, err := GetDomain(c)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if domain != test.expected {
				t.Errorf("Expected domain %q, got %q", test.expected, domain)
			}
		})
	}
}
