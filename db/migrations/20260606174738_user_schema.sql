-- +goose Up

CREATE SCHEMA IF NOT EXISTS user_schema;

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE user_schema.profiles (
    id          UUID PRIMARY KEY,
    display_name VARCHAR(50)  NOT NULL,
    username     VARCHAR(30)  NOT NULL,
    avatar_url   TEXT,
    bio          VARCHAR(500),
    last_seen_at TIMESTAMPTZ,
    is_active    BOOLEAN      NOT NULL DEFAULT true,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_profiles_username
    ON user_schema.profiles (username);

CREATE INDEX idx_profiles_display_name_trgm
    ON user_schema.profiles USING gin (display_name gin_trgm_ops);

CREATE INDEX idx_profiles_username_trgm
    ON user_schema.profiles USING gin (username gin_trgm_ops);

CREATE INDEX idx_profiles_is_active
    ON user_schema.profiles (is_active)
    WHERE is_active = true;

CREATE INDEX idx_profiles_created_at
    ON user_schema.profiles (created_at DESC);

-- +goose Down

DROP TABLE IF EXISTS user_schema.profiles;
DROP SCHEMA IF EXISTS user_schema CASCADE;