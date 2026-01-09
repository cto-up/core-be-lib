package service

import (
	"context"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db/repository"
)

type FullUser struct {
	Disabled      bool   `json:"disabled"`
	EmailVerified bool   `json:"email_verified"`
	Email         string `json:"email"`
	core.User
}

// UserCreatedCallback is an optional callback function that is called after a user is successfully created.
// It receives the context, tenant ID, and the created user.
type UserCreatedCallback func(ctx context.Context, tenantID string, user repository.CoreUser)

// UserEventInitFunc is a function that initializes the user event callback in a UserService
type UserEventInitFunc func(userService UserService)

var (
	userEventInitFunc UserEventInitFunc
)

// SetUserEventInitFunc sets a global function that will be called when a UserService is created
// This allows external modules (like realtime) to register their event callbacks
func SetUserEventInitFunc(fn UserEventInitFunc) {
	userEventInitFunc = fn
}

// GetUserEventInitFunc returns the global user event init function
func GetUserEventInitFunc() UserEventInitFunc {
	return userEventInitFunc
}

func convertToRoleDTOs(dbRoles []string) []core.Role {
	roles := make([]core.Role, len(dbRoles))
	for i, role := range dbRoles {
		roles[i] = core.Role(role)
	}
	return roles
}
func convertToRoles(roles []core.Role) []string {
	dbRoles := make([]string, len(roles))
	for i, role := range roles {
		dbRoles[i] = string(role)
	}
	return dbRoles
}
