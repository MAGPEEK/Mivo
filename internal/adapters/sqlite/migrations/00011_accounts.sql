-- Accounts, profiles, library access and sign-in sessions (ADR 0032 §1, §3, §7; task M7-08).
-- Additive: no existing table changes. ADR 0004 conventions apply: timestamps are INTEGER
-- Unix milliseconds in UTC, IDs are text UUIDs chosen by the caller, every table is STRICT.
--
-- Hashes (argon2id PHC strings for passwords and PINs, SHA-256 of session tokens) are stored
-- here and nowhere else; the adapter never logs them.
--
-- Library access is "all libraries" (all_libraries = 1, including libraries added later) or
-- the rows of user_libraries / profile_libraries (all_libraries = 0; no rows allow nothing).
-- Deleting a library removes its access rows by cascade, so a list never names a library
-- that does not exist.
--
-- Content limit of a profile: max_age NULL means no age limit; hide_unrated hides titles
-- without a known rating.
--
-- Invariant for later writers: auth_sessions.profile_id, when set, must be a profile of the
-- session's user_id; the schema does not enforce it (M7-09 checks it in its queries).

-- +goose Up
CREATE TABLE users (
    id            TEXT    NOT NULL PRIMARY KEY,
    name          TEXT    NOT NULL CHECK (name <> ''),
    -- account.NameKey(name): names are unique case-insensitively.
    name_key      TEXT    NOT NULL UNIQUE CHECK (name_key <> ''),
    role          TEXT    NOT NULL CHECK (role IN ('admin', 'user')),
    disabled      INTEGER NOT NULL DEFAULT 0 CHECK (disabled IN (0, 1)),
    password_hash TEXT    NOT NULL CHECK (password_hash <> ''),
    all_libraries INTEGER NOT NULL DEFAULT 1 CHECK (all_libraries IN (0, 1)),
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
) STRICT;

CREATE TABLE profiles (
    id            TEXT    NOT NULL PRIMARY KEY,
    user_id       TEXT    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name          TEXT    NOT NULL CHECK (name <> ''),
    is_primary    INTEGER NOT NULL CHECK (is_primary IN (0, 1)),
    avatar_color  TEXT    NOT NULL CHECK (avatar_color <> ''),
    pin_hash      TEXT    CHECK (pin_hash IS NULL OR pin_hash <> ''),
    max_age       INTEGER CHECK (max_age IS NULL OR max_age IN (0, 6, 12, 16, 18)),
    hide_unrated  INTEGER NOT NULL DEFAULT 0 CHECK (hide_unrated IN (0, 1)),
    -- For a managed profile "all" means the account's set (ADR 0032 §7).
    all_libraries INTEGER NOT NULL DEFAULT 1 CHECK (all_libraries IN (0, 1)),
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
) STRICT;

-- Serves the user_id foreign key and lists the profiles of an account.
CREATE INDEX profiles_by_user ON profiles (user_id, is_primary);

-- Exactly one primary profile per account: the store inserts it with the account, and this
-- index refuses a second one.
CREATE UNIQUE INDEX profiles_one_primary ON profiles (user_id) WHERE is_primary = 1;

CREATE TABLE user_libraries (
    user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    library_id TEXT NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, library_id)
) STRICT, WITHOUT ROWID;

-- Serves the library_id foreign key, so deleting a library does not scan the table.
CREATE INDEX user_libraries_by_library ON user_libraries (library_id);

CREATE TABLE profile_libraries (
    profile_id TEXT NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    library_id TEXT NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    PRIMARY KEY (profile_id, library_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX profile_libraries_by_library ON profile_libraries (library_id);

-- A sign-in session is also the device (ADR 0032 §3). Only the SHA-256 of the token is kept.
-- Deleting the selected profile leaves the session signed in without a profile.
CREATE TABLE auth_sessions (
    id            TEXT    NOT NULL PRIMARY KEY,
    token_hash    TEXT    NOT NULL UNIQUE CHECK (token_hash <> ''),
    user_id       TEXT    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    profile_id    TEXT    REFERENCES profiles (id) ON DELETE SET NULL,
    remember      INTEGER NOT NULL DEFAULT 0 CHECK (remember IN (0, 1)),
    device_name   TEXT    NOT NULL,
    client        TEXT    NOT NULL CHECK (client IN ('web', 'tv', 'cli')),
    platform      TEXT    NOT NULL DEFAULT '',
    address_class TEXT    NOT NULL CHECK (address_class IN ('lan', 'remote')),
    created_at    INTEGER NOT NULL,
    last_seen_at  INTEGER NOT NULL
) STRICT;

-- Serves the user_id foreign key and the per-account device list and last activity.
CREATE INDEX auth_sessions_by_user ON auth_sessions (user_id, last_seen_at);

-- Serves the profile_id foreign key (ON DELETE SET NULL).
CREATE INDEX auth_sessions_by_profile ON auth_sessions (profile_id);

-- +goose Down
DROP TABLE auth_sessions;
DROP TABLE profile_libraries;
DROP TABLE user_libraries;
DROP TABLE profiles;
DROP TABLE users;
