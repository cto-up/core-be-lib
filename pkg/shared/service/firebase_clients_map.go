package service

import (
	"context"
	"sync"

	"firebase.google.com/go/auth"
)

var AuthClient *auth.Client

type FirebaseTenantClient struct {
	TenantID string
	Client   *auth.TenantClient
}

type BaseAuthClient interface {
	CreateUser(ctx context.Context, user *auth.UserToCreate) (*auth.UserRecord, error)
	UpdateUser(
		ctx context.Context, uid string, user *auth.UserToUpdate) (ur *auth.UserRecord, err error)
	DeleteUser(ctx context.Context, uid string) error
	GetUser(ctx context.Context, uid string) (*auth.UserRecord, error)
	SetCustomUserClaims(ctx context.Context, uid string, customClaims map[string]interface{}) error

	EmailVerificationLink(ctx context.Context, email string) (string, error)
	PasswordResetLink(ctx context.Context, email string) (string, error)
	EmailVerificationLinkWithSettings(
		ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error)
	PasswordResetLinkWithSettings(
		ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error)
	EmailSignInLink(
		ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error)
	VerifyIDToken(ctx context.Context, idToken string) (*auth.Token, error)
}

// FirebaseTenantClientConnectionPool manages a pool of Firebase tenant clients
type FirebaseTenantClientConnectionPool struct {
	multitenantService *MultitenantService
	cli                *auth.Client
	clients            map[string]*FirebaseTenantClient
	mu                 sync.RWMutex
}

// NewFirebaseTenantClientConnectionPool creates a new connection pool
func NewFirebaseTenantClientConnectionPool(ctx context.Context, multitenantService *MultitenantService) (*FirebaseTenantClientConnectionPool, error) {
	client, err := newFirebaseClient(ctx)
	if err != nil {
		return nil, err
	}
	return &FirebaseTenantClientConnectionPool{
		multitenantService: multitenantService,
		cli:                client,
		clients:            make(map[string]*FirebaseTenantClient),
	}, nil
}

func (p *FirebaseTenantClientConnectionPool) GetClient() *auth.Client {
	return p.cli
}

// GetFirebaseTenantClient retrieves or creates a Firebase tenant client
func (p *FirebaseTenantClientConnectionPool) GetTenantClient(ctx context.Context, subdomain string) (*FirebaseTenantClient, error) {
	// get tenant from context using subdomain
	tenantID, err := p.multitenantService.GetFirebaseTenantID(ctx, subdomain)
	if err != nil {
		return nil, err
	}
	return p.GetTenantClientByTenantID(tenantID)
}

func (p *FirebaseTenantClientConnectionPool) GetTenantClientByTenantID(tenantID string) (*FirebaseTenantClient, error) {
	p.mu.RLock()
	client, exists := p.clients[tenantID]
	p.mu.RUnlock()

	if exists {
		return client, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring the write lock
	if client, exists = p.clients[tenantID]; exists {
		return client, nil
	}

	// Create a new Firebase app
	tenantClient, err := p.cli.TenantManager.AuthForTenant(tenantID)
	if err != nil {
		return nil, err
	}

	client = &FirebaseTenantClient{
		TenantID: tenantID,
		Client:   tenantClient,
	}

	p.clients[tenantID] = client
	return client, nil
}

// RemoveFirebaseTenantClient removes a Firebase tenant client from the pool
func (p *FirebaseTenantClientConnectionPool) RemoveFirebaseTenantClient(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.clients, name)
}

// Get BaseAuthClient for a given tenant based on Subdomain
func (p *FirebaseTenantClientConnectionPool) GetBaseAuthClient(ctx context.Context, subdomain string) (BaseAuthClient, error) {
	// get tenant from context using subdomain
	tenantID, err := p.multitenantService.GetFirebaseTenantID(ctx, subdomain)
	if err != nil {
		return nil, err
	}
	return p.GetBaseAuthClientForTenant(tenantID)
}

func (p *FirebaseTenantClientConnectionPool) GetBaseAuthClientForTenant(tenantID string) (BaseAuthClient, error) {
	if tenantID == "" {
		return p.GetClient(), nil
	} else {
		firebaseTenantClient, err := p.GetTenantClientByTenantID(tenantID)
		if err != nil {
			return nil, err
		}
		return firebaseTenantClient.Client, nil
	}
}
