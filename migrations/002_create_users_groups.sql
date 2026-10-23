CREATE TABLE IF NOT EXISTS groups (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- New users are put in the "admin" group at signup, so it must exist.
INSERT INTO groups (name) VALUES ('admin')
ON CONFLICT (name) DO NOTHING;

-- Emails are stored lower-cased by the app, so a plain UNIQUE is enough.
CREATE TABLE IF NOT EXISTS users (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    email VARCHAR(254) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    group_id BIGINT NOT NULL REFERENCES groups(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
