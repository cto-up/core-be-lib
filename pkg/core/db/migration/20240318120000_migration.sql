-- +goose Up
-- hack to make to allow sqlc to generate code
-- core_migrations definition

CREATE TABLE IF NOT EXISTS core_migrations (
	"version" int8 NOT NULL,
	dirty bool NOT NULL,
	CONSTRAINT core_migrations_pkey PRIMARY KEY (version)
);
-- +goose Down
-- Drop table

-- DROP TABLE core_migrations;
