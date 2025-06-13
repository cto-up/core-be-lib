package handlers

import (
	"ctoup.com/coreapp/api/health"
	core "ctoup.com/coreapp/pkg/core/api"
	config "ctoup.com/coreapp/pkg/core/config/api"
	"ctoup.com/coreapp/pkg/core/db"
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
	*core.RoleHandler
	*core.ClientApplicationHandler
	*core.TranslationHandler
}

func CreateCoreHandlers(connPool *pgxpool.Pool, authClientPool *access.FirebaseTenantClientConnectionPool, multiTenantService *access.MultitenantService, clientAppService *access.ClientApplicationService) Handlers {
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
		RoleHandler:              core.NewRoleHandler(store),
		ClientApplicationHandler: core.NewClientApplicationHandler(store, clientAppService),
		TranslationHandler:       core.NewTranslationHandler(store),
	}
	return handlers
}
