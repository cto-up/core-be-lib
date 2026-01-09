package service

import (
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/shared/auth"
)

// UserServiceStrategyFactory creates the appropriate strategy based on provider
type UserServiceStrategyFactory struct{}

func (f *UserServiceStrategyFactory) CreateUserServiceStrategy(store *db.Store, authClientPool auth.AuthClientPool) UserService {
	providerName := authClientPool.GetProviderName()

	switch providerName {
	case "kratos":
		return NewSharedUserService(store, authClientPool)
	case "firebase":
		return NewIsolatedUserService(store, authClientPool)
	default:
		// Default to Firebase for backward compatibility
		return NewIsolatedUserService(store, authClientPool)
	}
}

func NewUserServiceStrategyFactory() *UserServiceStrategyFactory {
	return &UserServiceStrategyFactory{}
}
