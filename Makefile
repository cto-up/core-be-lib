include .env
export $(shell sed 's/=.*//' .env)
DB_CONNECTION = postgres://${DATABASE_USERNAME}:${DATABASE_PASSWORD}@${DATABASE_URL}
COMMAND ?= new # new:front_views
FILE ?= entity.json

testme:
	env

kratosup:
	docker compose -f docker/docker-compose-kratos.yml up

kratosdown:
	docker compose -f docker/docker-compose-kratos.yml down

postgresup:
	docker compose -f docker/docker-compose-postgresql.yml up

postgresdown:
	docker compose -f docker/docker-compose-postgresql.yml down

sqlc:
	cd pkg/core/db; echo "I'm in backend core"; \
	sqlc generate

BASE_API_BE_DIR := api/openapi
BASE_API_FE_DIR := ../core-fe-lib/lib/openapi

# Define the pattern to search for and replace
SEARCH_STRING_1 := from \'./core
REPLACE_STRING_1 := from \'openapi/core/core

SEARCH_STRING_2 := from \'../core
REPLACE_STRING_2 := from \'openapi/core/core

BASE_OPENAPI_CORE_DIR := pkg/core/api/openapi
BASE_MODULE_DIR := internal/modules

openapi:
	@echo "Generating Core OpenAPI code"
	@rm -rf $(BASE_API_FE_DIR)/core
	openapi --input $(BASE_OPENAPI_CORE_DIR)/core-api.yaml --output $(BASE_API_FE_DIR)/core --client axios
	oapi-codegen -config $(BASE_OPENAPI_CORE_DIR)/parts/_oapi-schema-config.yaml $(BASE_OPENAPI_CORE_DIR)/core-schema.yaml
	oapi-codegen -config $(BASE_OPENAPI_CORE_DIR)/parts/_oapi-service-config.yaml $(BASE_OPENAPI_CORE_DIR)/core-api.yaml

release:
	@echo "Creating release"
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION parameter is required. Use 'vx.x.x' format."; \
		exit 1; \
	fi; \
	gh release create $(VERSION) --title "$(VERSION)" --notes "$(NOTES)"

include .env
export $(shell sed 's/=.*//' .env)
DB_CONNECTION = postgres://${DATABASE_USERNAME}:${DATABASE_PASSWORD}@${DATABASE_URL}

.PHONY: postgresup postgresdown sqlc test openapi

# Library migrations are 16 digits: YYYYMMDDHHMMSS + source id 01. Consumers
# flatten every library's migrations and their own modules into ONE goose
# namespace, so a bare timestamp can collide with an app migration — which is
# exactly how coreapp v0.2.29 collided with skeells on 20260810120000. The
# 2-digit suffix makes a cross-repo collision arithmetically impossible.
new-migration: ## Create a goose migration (NAME=<snake_case>)
	@if [ -z "$(NAME)" ]; then \
		echo "Error: NAME is required. Example: make new-migration NAME=add_user_index"; \
		exit 1; \
	fi
	@VERSION="$$(date -u +%Y%m%d%H%M%S)01"; \
	FILE="pkg/core/db/migration/$${VERSION}_$(NAME).sql"; \
	if [ -e "$$FILE" ]; then echo "Error: $$FILE already exists."; exit 1; fi; \
	printf -- '-- +goose Up\n\n\n-- +goose Down\n\n' > "$$FILE"; \
	echo "Created $$FILE"
	@./scripts/check-migration-versions.sh
