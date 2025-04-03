include .env
export $(shell sed 's/=.*//' .env)
DB_CONNECTION = postgres://${DATABASE_USERNAME}:${DATABASE_PASSWORD}@${DATABASE_URL}
COMMAND ?= new # new:front_views
FILE ?= entity.json

testme:
	env

postgresup:
	docker compose -f docker/postgresql.yml up

postgresdown:
	docker compose -f docker/postgresql.yml down

migrateup:
	@$(MAKE) migrate-module DIRECTION=up
migrateup1:
	@$(MAKE) migrate-module DIRECTION=up STEP=1
migratedown:
	@$(MAKE) migrate-module DIRECTION=down
migratedown1:
	@$(MAKE) migrate-module DIRECTION=down STEP=1

migrate-module:
	cd pkg/core/db; \
	echo "I'm in pkg/core/db and $(DIRECTION) and $(STEP)"; \
	migrate -path migration -database "${DB_CONNECTION}&x-migrations-table=core_migrations" -verbose $(DIRECTION) $(STEP)

sqlc:
	cd pkg/core/db; echo "I'm in backend core"; \
	sqlc generate

BASE_API_BE_DIR := api/openapi
BASE_API_FE_DIR := frontend/openapi

openapi:
	# reads the OpenAPI specification file core-api.yaml, removes the default responses from all paths, and then writes the modified content to a new file named core-fe-api.yaml in the same directory. 
	yq 'del(.paths.[].[].responses.default)' ./openapi/core-api.yaml > ./openapi/core-fe-api.yaml
	openapi --input ./openapi/core-fe-api.yaml --output ./frontend/src/openapi --client axios
	rm ./openapi/core-fe-api.yaml
	rm -f $(BASE_API_BE_DIR)/*.go
	oapi-codegen -generate gin,types -package openapi openapi/core-api.yaml > $(BASE_API_BE_DIR)/api.go


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

include .env
export $(shell sed 's/=.*//' .env)
DB_CONNECTION = postgres://${DATABASE_USERNAME}:${DATABASE_PASSWORD}@${DATABASE_URL}

.PHONY: postgresup postgresdown migratecreate migrateup migratedown sqlc test openapi
