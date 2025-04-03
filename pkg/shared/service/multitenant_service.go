package service

import (
	"errors"
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

	orgin := c.Request.Header.Get("Origin")

	parsedURL, err := url.Parse(orgin)
	if err != nil {
		return nil, errors.New("Error parsing URL: " + err.Error())
	}

	// Extract the host (e.g., corpa.moncto.test:9001)
	return parsedURL, nil
}

func GetSubdomain(c *gin.Context) (string, error) {

	host, err := GetHost(c)
	if err != nil {
		return "", err
	}

	// Split the host by '.'
	hostParts := strings.Split(host.Host, ".")

	// Assuming the subdomain is the first part (example: sub.example.com)
	if len(hostParts) > 2 {
		return hostParts[0], nil
	} else {
		return "", nil
	}
}
func GetDomain(c *gin.Context) (string, error) {

	host, err := GetHost(c)
	if err != nil {
		return "", err
	}

	// Split the host by '.'
	hostParts := strings.SplitN(host.Host, ".", 2)

	// Assuming the subdomain is the first part (example: sub.example.com)
	if len(hostParts) == 2 {
		return hostParts[1], nil
	} else {
		return hostParts[0], nil
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
