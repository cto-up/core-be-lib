-- +goose Up
-- core_roles definition

CREATE TABLE core_roles (
	id uuid NOT NULL DEFAULT gen_random_uuid(),
	user_id varchar NOT NULL,
	created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
	updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
	"name" VARCHAR(255) NOT NULL UNIQUE,
	CONSTRAINT roles_pk PRIMARY KEY (id)
);

CREATE TRIGGER update_roles_modtime
BEFORE UPDATE ON core_roles
FOR EACH ROW
EXECUTE FUNCTION update_modified_column();

-- +goose Down
-- core_roles definition

-- Drop table

DROP TABLE if exists core_roles;
