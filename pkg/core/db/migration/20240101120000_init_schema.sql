-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_modified_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = clock_timestamp();
    RETURN NEW;   
END;
$$ language 'plpgsql';
-- +goose StatementEnd

-- core_users definition

CREATE TABLE core_users (
	-- user id provided by the identity provider
	id varchar NOT NULL UNIQUE,
	"profile" jsonb,
	email varchar(254) NULL,
	core_roles uuid[] NULL,
	created_at timestamptz NOT NULL DEFAULT now(),
	tenant_id varchar(64),
	CONSTRAINT users_pk PRIMARY KEY (id)
);

-- +goose Down
-- core_users definition

-- Drop table

DROP TABLE if exists core_users;
