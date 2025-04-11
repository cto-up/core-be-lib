package handlers

import (
	"ctoup.com/coreapp/api/health"
	core "ctoup.com/coreapp/pkg/core/api"
	config "ctoup.com/coreapp/pkg/core/config/api"
	"ctoup.com/coreapp/pkg/core/db"
	access "ctoup.com/coreapp/pkg/shared/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateCoreHandlers(connPool *pgxpool.Pool, authClientPool *access.FirebaseTenantClientConnectionPool, multiTenantService *access.MultitenantService, clientAppService *access.ClientApplicationService) Handlers {
	store := db.NewStore(connPool)
	handlers := Handlers{
		GlobalConfigHandler:      config.NewGlobalConfigHandler(store, authClientPool),
		TenantConfigHandler:      config.NewTenantConfigHandler(store, authClientPool),
		PromptHandler:            core.NewPromptHandler(store, authClientPool),
		HealthHandler:            health.NewHealthHandler(store),
		TenantHandler:            core.NewTenantHandler(store, authClientPool, multiTenantService),
		UserHandler:              core.NewUserHandler(store, authClientPool),
		UserSuperAdminHandler:    core.NewUserSuperAdminHandler(store, authClientPool),
		RoleHandler:              core.NewRoleHandler(store),
		ClientApplicationHandler: core.NewClientApplicationHandler(store, clientAppService),
	}
	return handlers
}

type Handlers struct {
	*config.GlobalConfigHandler
	*core.PromptHandler
	*config.TenantConfigHandler
	*health.HealthHandler
	*core.TenantHandler
	*core.UserHandler
	*core.UserSuperAdminHandler
	*core.RoleHandler
	*core.ClientApplicationHandler
}
