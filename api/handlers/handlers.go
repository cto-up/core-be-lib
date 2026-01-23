package handlers

import (
	"ctoup.com/coreapp/api/health"
	core "ctoup.com/coreapp/pkg/core/api"
	config "ctoup.com/coreapp/pkg/core/config/api"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/shared/auth"
	access "ctoup.com/coreapp/pkg/shared/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handlers struct {
	*config.GlobalConfigHandler
	*core.PromptHandler
	*config.TenantConfigHandler
	*health.HealthHandler
	*core.TenantHandler
	*core.UserHandler
	*core.UserAdminHandler
	*core.UserSuperAdminHandler
	*core.SuperAdminHandler
	*core.ClientApplicationHandler
	*core.TranslationHandler
	*core.MigrationHandler
	*core.RecoveryHandler
	*core.MFAHandler
}

func CreateCoreHandlers(connPool *pgxpool.Pool, authClientPool auth.AuthProvider, multiTenantService *access.MultitenantService, clientAppService *access.ClientApplicationService) Handlers {
	store := db.NewStore(connPool)
	handlers := Handlers{
		GlobalConfigHandler:      config.NewGlobalConfigHandler(store, authClientPool),
		TenantConfigHandler:      config.NewTenantConfigHandler(store, authClientPool),
		PromptHandler:            core.NewPromptHandler(store, authClientPool),
		HealthHandler:            health.NewHealthHandler(store),
		TenantHandler:            core.NewTenantHandler(store, authClientPool, multiTenantService),
		UserHandler:              core.NewUserHandler(store, authClientPool),
		UserAdminHandler:         core.NewUserAdminHandler(store, authClientPool),
		UserSuperAdminHandler:    core.NewUserSuperAdminHandler(store, authClientPool),
		SuperAdminHandler:        core.NewSuperAdminHandler(authClientPool),
		ClientApplicationHandler: core.NewClientApplicationHandler(store, clientAppService),
		TranslationHandler:       core.NewTranslationHandler(store),
		MigrationHandler:         core.NewMigrationHandler(store),
		RecoveryHandler:          core.NewRecoveryHandler(authClientPool),
		MFAHandler:               core.NewMFAHandler(authClientPool),
	}
	return handlers
}
