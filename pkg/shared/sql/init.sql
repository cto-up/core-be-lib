CREATE EXTENSION vector;
CREATE TABLE blog (id bigserial PRIMARY KEY, text_data varchar,  embedding vector(1536));