-- Sign-in session queries (ADR 0032 section 3, task M7-09). A session row is also the device.
-- Invariant: auth_sessions.profile_id, when set, is a profile of the session's user_id. The
-- schema cannot express it, so every query that writes profile_id checks it, and the resolve
-- query joins the profile on both columns.

-- name: ProfileBelongsToUser :one
SELECT COUNT(*) FROM profiles
WHERE id = sqlc.arg(profile_id) AND user_id = sqlc.arg(user_id);

-- name: InsertSession :exec
INSERT INTO auth_sessions (id, token_hash, user_id, profile_id, remember, device_name, client, platform,
                           address_class, created_at, last_seen_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- GetSessionByTokenHash resolves a token hash to the session, its account (disabled ones too;
-- the caller decides) and the selected profile in one statement. The profile columns are NULL
-- when no profile is selected.
-- name: GetSessionByTokenHash :one
SELECT auth_sessions.id, auth_sessions.token_hash, auth_sessions.user_id, auth_sessions.remember,
       auth_sessions.device_name, auth_sessions.client, auth_sessions.platform, auth_sessions.address_class,
       auth_sessions.created_at, auth_sessions.last_seen_at,
       users.name AS user_name, users.role AS user_role, users.disabled AS user_disabled,
       profiles.id AS profile_id, profiles.name AS profile_name, profiles.is_primary AS profile_is_primary,
       profiles.avatar_color AS profile_avatar_color, profiles.pin_hash IS NOT NULL AS profile_has_pin
FROM auth_sessions
JOIN users ON users.id = auth_sessions.user_id
LEFT JOIN profiles ON profiles.id = auth_sessions.profile_id AND profiles.user_id = auth_sessions.user_id
WHERE auth_sessions.token_hash = ?;

-- TouchSession writes last_seen_at and the address class only when the stored value is at
-- least the touch interval old or the address class changed, so frequent requests (HLS
-- segments) do not turn into a write each.
-- name: TouchSession :execrows
UPDATE auth_sessions
SET last_seen_at = sqlc.arg(seen_at), address_class = sqlc.arg(address_class)
WHERE id = sqlc.arg(id)
  AND (last_seen_at <= sqlc.arg(stale_before) OR address_class <> sqlc.arg(address_class));

-- SetSessionProfile selects a profile; it changes nothing when the profile belongs to another
-- account.
-- name: SetSessionProfile :execrows
UPDATE auth_sessions
SET profile_id = sqlc.arg(profile_id), remember = sqlc.arg(remember)
WHERE auth_sessions.id = sqlc.arg(id)
  AND EXISTS (SELECT 1 FROM profiles
              WHERE profiles.id = sqlc.arg(profile_id) AND profiles.user_id = auth_sessions.user_id);

-- name: ClearSessionProfile :execrows
UPDATE auth_sessions
SET profile_id = NULL, remember = 0
WHERE id = ?;

-- name: ListUserSessions :many
SELECT id, token_hash, user_id, profile_id, remember, device_name, client, platform, address_class,
       created_at, last_seen_at
FROM auth_sessions
WHERE user_id = ?
ORDER BY last_seen_at DESC, id;

-- name: ListAllSessions :many
SELECT auth_sessions.id, auth_sessions.token_hash, auth_sessions.user_id, auth_sessions.profile_id,
       auth_sessions.remember, auth_sessions.device_name, auth_sessions.client, auth_sessions.platform,
       auth_sessions.address_class, auth_sessions.created_at, auth_sessions.last_seen_at,
       users.name AS user_name
FROM auth_sessions
JOIN users ON users.id = auth_sessions.user_id
ORDER BY auth_sessions.last_seen_at DESC, auth_sessions.id;

-- name: DeleteSession :many
DELETE FROM auth_sessions
WHERE id = ?
RETURNING token_hash;

-- name: DeleteUserSession :many
DELETE FROM auth_sessions
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id)
RETURNING token_hash;

-- name: DeleteUserSessionsExcept :many
DELETE FROM auth_sessions
WHERE user_id = sqlc.arg(user_id) AND id <> sqlc.arg(keep_id)
RETURNING token_hash;

-- DeleteExpiredSessions deletes sessions idle or alive at least as long as the bounds allow;
-- the bounds themselves count as expired, like the resolver does.
-- name: DeleteExpiredSessions :many
DELETE FROM auth_sessions
WHERE last_seen_at <= sqlc.arg(idle_before) OR created_at <= sqlc.arg(created_before)
RETURNING token_hash;
