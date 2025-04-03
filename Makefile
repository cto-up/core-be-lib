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

MIGRATENAME ?= $(shell bash -c 'read -p "Migratename: " migratename; echo $$migratename')

migratecreate:
	cd backend/src/db; echo "I'm in backend"; \
	migrate create -ext sql -dir migration -seq $(MIGRATENAME)

migrateup-core:
	@$(MAKE) migrate-module FOLDER=core DB_PREFIX=core DIRECTION=up
migrateup-core-1:
	@$(MAKE) migrate-module FOLDER=core DB_PREFIX=core DIRECTION=up STEP=1
migratedown-core:
	@$(MAKE) migrate-module FOLDER=core DB_PREFIX=core DIRECTION=down
migratedown-core-1:
	@$(MAKE) migrate-module FOLDER=core DB_PREFIX=core DIRECTION=down STEP=1

sqlc-module:
	cd backend/src/internal/$(FOLDER)/db; echo "I'm in backend $(MODULE)"; \
	sqlc generate
sqlc-core:
	@$(MAKE) sqlc-module FOLDER=core MODULE=core

openapi:
	# reads the OpenAPI specification file core-api.yaml, removes the default responses from all paths, and then writes the modified content to a new file named core-fe-api.yaml in the same directory. 
	yq 'del(.paths.[].[].responses.default)' ./openapi/core-api.yaml > ./openapi/core-fe-api.yaml
	openapi --input ./openapi/core-fe-api.yaml --output ./frontend/src/openapi --client axios
	rm ./openapi/core-fe-api.yaml
	rm -f backend/src/api/openapi/*.go
	oapi-codegen -generate gin,types -package openapi openapi/core-api.yaml > backend/src/api/openapi/api.go

BASE_API_BE_DIR := api/openapi
BASE_API_FE_DIR := frontend/openapi

# Define the pattern to search for and replace
SEARCH_STRING_1 := from \'./core
REPLACE_STRING_1 := from \'openapi/core/core

SEARCH_STRING_2 := from \'../core
REPLACE_STRING_2 := from \'openapi/core/core

BASE_OPENAPI_CORE_DIR := internal/core/api/openapi
BASE_MODULE_DIR := internal/modules

openapi-core:
	@echo "Generating Core OpenAPI code"
	@rm -rf $(BASE_API_FE_DIR)/core
	openapi --input $(BASE_OPENAPI_CORE_DIR)/core-api.yaml --output $(BASE_API_FE_DIR)/core --client axios
	oapi-codegen -config $(BASE_OPENAPI_CORE_DIR)/parts/_oapi-schema-config.yaml $(BASE_OPENAPI_CORE_DIR)/core-schema.yaml
	oapi-codegen -config $(BASE_OPENAPI_CORE_DIR)/parts/_oapi-service-config.yaml $(BASE_OPENAPI_CORE_DIR)/core-api.yaml

# openapi adds core elements which are duplicated in the core-api.yaml file,
# so we can remove them from the generated code
openapi-module:
	@echo "Generating OpenAPI code for context: $(MODULE)..."
	@rm -rf $(BASE_API_FE_DIR)/$(MODULE)
	openapi --input $(BASE_MODULE_DIR)/$(MODULE)/api/openapi/$(MODULE)-api.yaml --output $(BASE_API_FE_DIR)/$(MODULE) --client axios
	@rm -rf $(BASE_API_FE_DIR)/$(MODULE)/core
	@find $(BASE_API_FE_DIR)/$(MODULE) -name "*.ts" -type f -exec sed -i '' "s|$(SEARCH_STRING_1)|$(REPLACE_STRING_1)|g" {} +
	@find $(BASE_API_FE_DIR)/$(MODULE) -name "*.ts" -type f -exec sed -i '' "s|$(SEARCH_STRING_2)|$(REPLACE_STRING_2)|g" {} +
	@echo "Replacement complete."
	oapi-codegen -config $(BASE_MODULE_DIR)/$(MODULE)/api/openapi/_oapi-schema-config.yaml $(BASE_MODULE_DIR)/$(MODULE)/api/openapi/$(MODULE)-schema.yaml
	oapi-codegen -generate gin,types -package $(MODULE) $(BASE_MODULE_DIR)/$(MODULE)/api/openapi/$(MODULE)-api.yaml > $(BASE_API_BE_DIR)/$(MODULE)/$(MODULE)-service.go	
	
openapi-contact:
	@$(MAKE) openapi-module MODULE=contact
openapi-meeting:
	@$(MAKE) openapi-module MODULE=meeting
openapi-learning:
	@$(MAKE) openapi-module MODULE=learning
openapi-project:
	@$(MAKE) openapi-module MODULE=project
openapi-document:
	@$(MAKE) openapi-module MODULE=document
openapi-teamwork:
	@$(MAKE) openapi-module MODULE=teamwork
openapi-automation:
	@$(MAKE) openapi-module MODULE=automation
openapi-recruitment:
	@$(MAKE) openapi-module MODULE=recruitment
openapi-skeellscoach:
	@$(MAKE) openapi-module MODULE=skeellscoach

include .env
export $(shell sed 's/=.*//' .env)
DB_CONNECTION = postgres://${DATABASE_USERNAME}:${DATABASE_PASSWORD}@${DATABASE_URL}

.PHONY: postgresup postgresdown migratecreate migrateup migratedown sqlc test proto openapi hygen deploy provision clearactions
