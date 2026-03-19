package service

import (
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/shared/auth"
)

// UserServiceStrategyFactory creates the appropriate strategy based on provider
type UserServiceStrategyFactory struct{}

func (f *UserServiceStrategyFactory) CreateUserServiceStrategy(store *db.Store, authClientPool auth.AuthClientPool) UserService {
	return NewSharedUserService(store, authClientPool)
}

func NewUserServiceStrategyFactory() *UserServiceStrategyFactory {
	return &UserServiceStrategyFactory{}
}
