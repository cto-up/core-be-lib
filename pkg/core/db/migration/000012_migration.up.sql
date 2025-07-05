-- hack to make to allow sqlc to generate code
-- public.core_migrations definition

CREATE TABLE IF NOT EXISTS public.core_migrations (
	"version" int8 NOT NULL,
	dirty bool NOT NULL,
	CONSTRAINT core_migrations_pkey PRIMARY KEY (version)
);