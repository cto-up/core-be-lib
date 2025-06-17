package service

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"

	"ctoup.com/coreapp/pkg/core/db"
	"github.com/gin-gonic/gin"
)

type TenantMap struct {
	sync.RWMutex
	data map[string]string
}

var tenantMapInstance *TenantMap
var once sync.Once

// GetTenantMap returns the singleton instance of TenantMap
func GetTenantMap() *TenantMap {
	once.Do(func() {
		tenantMapInstance = &TenantMap{
			data: make(map[string]string),
		}
	})
	return tenantMapInstance
}

type MultitenantService struct {
	store *db.Store
}

func NewMultitenantService(store *db.Store) *MultitenantService {
	return &MultitenantService{store: store}
}
func GetHost(c *gin.Context) (*url.URL, error) {
	// Try Origin header first (most reliable for cross-origin requests)
	origin := c.Request.Header.Get("Origin")

	// If Origin is empty, construct from Host header
	if origin == "" {
		host := c.Request.Host
		if host == "" {
			return nil, errors.New("no host information available")
		}

		// Determine scheme more reliably
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
		// Also check for forwarded protocol headers (common in reverse proxy setups)
		if proto := c.Request.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = strings.ToLower(proto)
		} else if proto := c.Request.Header.Get("X-Forwarded-Protocol"); proto != "" {
			scheme = strings.ToLower(proto)
		}

		origin = fmt.Sprintf("%s://%s", scheme, host)
	}

	parsedURL, err := url.Parse(origin)
	if err != nil {
		return nil, fmt.Errorf("error parsing URL '%s': %w", origin, err)
	}

	// Validate that we have a proper hostname
	if parsedURL.Hostname() == "" {
		return nil, errors.New("invalid hostname in URL")
	}

	return parsedURL, nil
}

// Common multi-part TLDs that need special handling
var multiPartTLDs = map[string]bool{
	// Country code TLDs with second level
	"co.uk": true, "org.uk": true, "me.uk": true, "ltd.uk": true, "plc.uk": true,
	"com.au": true, "net.au": true, "org.au": true, "edu.au": true, "gov.au": true,
	"co.nz": true, "net.nz": true, "org.nz": true, "edu.nz": true, "govt.nz": true,
	"co.jp": true, "or.jp": true, "ne.jp": true, "ac.jp": true, "ad.jp": true,
	"com.br": true, "net.br": true, "org.br": true, "edu.br": true, "gov.br": true,
	"com.mx": true, "net.mx": true, "org.mx": true, "edu.mx": true, "gob.mx": true,
	"co.za": true, "net.za": true, "org.za": true, "edu.za": true, "gov.za": true,
	"co.in": true, "net.in": true, "org.in": true, "edu.in": true, "gov.in": true,
	"com.cn": true, "net.cn": true, "org.cn": true, "edu.cn": true, "gov.cn": true,
	"com.sg": true, "net.sg": true, "org.sg": true, "edu.sg": true, "gov.sg": true,
	"com.hk": true, "net.hk": true, "org.hk": true, "edu.hk": true, "gov.hk": true,
	"com.tw": true, "net.tw": true, "org.tw": true, "edu.tw": true, "gov.tw": true,
	"co.kr": true, "ne.kr": true, "or.kr": true, "ac.kr": true, "go.kr": true,
	"com.my": true, "net.my": true, "org.my": true, "edu.my": true, "gov.my": true,
	"co.th": true, "net.th": true, "org.th": true, "edu.th": true, "go.th": true,
	"com.ph": true, "net.ph": true, "org.ph": true, "edu.ph": true, "gov.ph": true,
	"com.pk": true, "net.pk": true, "org.pk": true, "edu.pk": true, "gov.pk": true,
	"com.bd": true, "net.bd": true, "org.bd": true, "edu.bd": true, "gov.bd": true,
	"com.np": true, "net.np": true, "org.np": true, "edu.np": true, "gov.np": true,
	"com.lk": true, "net.lk": true, "org.lk": true, "edu.lk": true, "gov.lk": true,
	// European TLDs
	"co.de": true, "com.de": true,
	"co.fr": true, "com.fr": true,
	"co.it": true, "com.it": true,
	"co.es": true, "com.es": true,
	"co.nl": true, "com.nl": true,
	"co.be": true, "com.be": true,
	"co.ch": true, "com.ch": true,
	"co.at": true, "com.at": true,
	// Other common ones
	"com.co": true, "net.co": true, "org.co": true,
	"com.ve": true, "net.ve": true, "org.ve": true,
	"com.ar": true, "net.ar": true, "org.ar": true,
	"com.pe": true, "net.pe": true, "org.pe": true,
	"com.cl": true, "net.cl": true, "org.cl": true,
	"com.ec": true, "net.ec": true, "org.ec": true,
	"com.uy": true, "net.uy": true, "org.uy": true,
	"com.py": true, "net.py": true, "org.py": true,
	"com.bo": true, "net.bo": true, "org.bo": true,
}

// DomainInfo holds the parsed domain information
type DomainInfo struct {
	subdomain string
	Domain    string
	TLD       string
	FullHost  string
	Port      string // The port number if present (e.g., "9999")
}

// isIPAddress checks if the hostname is an IP address
func isIPAddress(hostname string) bool {
	return net.ParseIP(hostname) != nil
}

// extractDomainParts extracts domain parts considering multi-part TLDs
func extractDomainParts(hostname string) (subdomain, domain, tld string) {
	if hostname == "" {
		return "", "", ""
	}

	// Handle IP addresses
	if isIPAddress(hostname) {
		return "", hostname, ""
	}

	parts := strings.Split(hostname, ".")
	numParts := len(parts)

	// Handle edge cases
	if numParts == 0 {
		return "", "", ""
	}
	if numParts == 1 {
		// Single part like "localhost"
		return "", parts[0], ""
	}

	// Check for multi-part TLDs (e.g., co.uk, com.au)
	var tldParts int = 1 // Default to single TLD part

	// Check if last two parts form a known multi-part TLD
	if numParts >= 2 {
		lastTwoParts := strings.Join(parts[numParts-2:], ".")
		if multiPartTLDs[strings.ToLower(lastTwoParts)] {
			tldParts = 2
		}
	}

	// For our specific case, we want the second-to-last part before the domain
	// For example, in "bo.corpa.cto.com", we want "corpa"

	// Calculate domain parts based on TLD structure
	switch {
	case numParts <= tldParts:
		// Not enough parts for a proper domain
		return "", hostname, ""
	case numParts == tldParts+1:
		// Just domain + TLD (e.g., "example.com" or "example.co.uk")
		if tldParts == 1 {
			return "", parts[0], parts[1]
		} else {
			return "", parts[0], strings.Join(parts[1:], ".")
		}
	case numParts == tldParts+2:
		// One subdomain part (e.g., "sub.example.com")
		// For our case, this is what we want to extract
		return parts[0], parts[1], strings.Join(parts[2:], ".")
	default:
		// Multiple subdomain parts (e.g., "bo.corpa.cto.com")
		// We want to extract "corpa" as the subdomain
		return parts[numParts-tldParts-2], parts[numParts-tldParts-1], strings.Join(parts[numParts-tldParts:], ".")
	}
}

// GetDomainInfo extracts comprehensive domain information
func GetDomainInfo(c *gin.Context) (*DomainInfo, error) {
	host, err := GetHost(c)
	if err != nil {
		return nil, err
	}

	hostname := strings.ToLower(strings.TrimSpace(host.Hostname()))
	port := host.Port()

	subdomain, domain, tld := extractDomainParts(hostname)

	// Build full domain (domain + TLD)
	fullDomain := domain
	if tld != "" {
		fullDomain = domain + "." + tld
	}

	return &DomainInfo{
		subdomain: subdomain,
		Domain:    fullDomain,
		TLD:       tld,
		FullHost:  hostname,
		Port:      port,
	}, nil
}

// GetSubdomain extracts just the subdomain part
func GetSubdomain(c *gin.Context) (string, error) {
	domainInfo, err := GetDomainInfo(c)
	if err != nil {
		return "", err
	}
	return domainInfo.subdomain, nil
}

// GetDomain extracts the domain (including TLD)
func GetDomain(c *gin.Context) (string, error) {
	domainInfo, err := GetDomainInfo(c)
	if err != nil {
		return "", err
	}
	return domainInfo.Domain, nil
}

func GetBaseDomainWithPort(c *gin.Context) (string, error) {
	domainInfo, err := GetDomainInfo(c)
	if err != nil {
		return "", err
	}

	baseDomain := domainInfo.Domain // This already gives "domain.com" or "domain.co.uk"

	if domainInfo.Port != "" {
		return fmt.Sprintf("%s:%s", baseDomain, domainInfo.Port), nil
	}
	return baseDomain, nil
}

// GetRootDomain extracts just the domain part without subdomain or port
func GetRootDomain(c *gin.Context) (string, error) {
	domainInfo, err := GetDomainInfo(c)
	if err != nil {
		return "", err
	}
	return domainInfo.Domain, nil
}

// GetTLD extracts just the top-level domain
func GetTLD(c *gin.Context) (string, error) {
	domainInfo, err := GetDomainInfo(c)
	if err != nil {
		return "", err
	}
	return domainInfo.TLD, nil
}

// Example usage and test cases
func ExampleUsage() {
	// Example test cases (you can remove this in production)
	testCases := []string{
		"sub.example.com",
		"api.v2.example.com",
		"www.example.co.uk",
		"sub.example.co.uk",
		"deep.nested.sub.example.com",
		"example.com",
		"example.co.uk",
		"localhost",
		"192.168.1.1",
		"api.staging.myapp.herokuapp.com",
	}

	for _, testCase := range testCases {
		parts := strings.Split(testCase, ".")
		subdomain, domain, tld := extractDomainParts(testCase)
		// Log results for testing
		_ = parts
		_ = subdomain
		_ = domain
		_ = tld
	}
}

// Map subdomain to Firebase tenant ID
func (uh *MultitenantService) GetFirebaseTenantID(c *gin.Context) (string, error) {
	subdomain, err := GetSubdomain(c)
	if err != nil {
		return "", err
	}
	if subdomain == "" || subdomain == "www" {
		return "", nil
	}

	tenantMap := GetTenantMap()

	tenantID, exists := tenantMap.data[subdomain]

	// If the subdomain is not found in the map, return an error
	if !exists {
		tenant, err := uh.store.GetTenantBySubdomain(c, subdomain)
		if err != nil {
			return "", err
		}

		tenantID = tenant.TenantID
		tenantMap.RLock()
		tenantMap.data[tenant.Subdomain] = tenantID
		tenantMap.RUnlock()
	}
	return tenantID, nil
}
