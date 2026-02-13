CREATE TYPE auth_type AS ENUM ('password', 'google', 'facebook', 'apple');
ALTER TABLE users
ADD COLUMN auth_type auth_type NOT NULL DEFAULT 'password';