CREATE TABLE IF NOT EXISTS organisations (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name VARCHAR(100) NOT NULL
);

-- Names are unique ignoring case ("Acme" and "acme" can't both exist),
-- while the stored name keeps the case it was given.
CREATE UNIQUE INDEX IF NOT EXISTS organisations_name_lower_key ON organisations (LOWER(name));
