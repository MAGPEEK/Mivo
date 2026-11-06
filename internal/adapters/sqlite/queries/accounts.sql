-- Account and profile queries (ADR 0032 sections 1 and 7, task M7-08). Sessions are M7-09.

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: CountEnabledAdmins :one
SELECT COUNT(*) FROM users
WHERE role = 'admin' AND disabled = 0;

-- name: InsertUser :exec
INSERT INTO users (id, name, name_key, role, disabled, password_hash, all_libraries, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateUser :execrows
UPDATE users
SET name = ?, name_key = ?, role = ?, disabled = ?, all_libraries = ?, updated_at = ?
WHERE id = ?;

-- name: SetUserPasswordHash :execrows
UPDATE users
SET password_hash = ?, updated_at = ?
WHERE id = ?;

-- DeleteUser removes the profiles, library access and sessions by cascade.
-- name: DeleteUser :execrows
DELETE FROM users
WHERE id = ?;

-- name: GetUser :one
SELECT id, name, name_key, role, disabled, password_hash, all_libraries, created_at, updated_at
FROM users
WHERE id = ?;

-- name: GetUserByNameKey :one
SELECT id, name, name_key, role, disabled, password_hash, all_libraries, created_at, updated_at
FROM users
WHERE name_key = ?;

-- ListUserSummaries returns every account with the content limit of its primary profile, its
-- number of profiles and its newest session activity (0 when it never signed in). The
-- correlated subqueries use the per-user indexes.
-- name: ListUserSummaries :many
SELECT users.id, users.name, users.role, users.disabled, users.all_libraries, users.created_at,
       primary_profile.max_age AS primary_max_age, primary_profile.hide_unrated AS primary_hide_unrated,
       (SELECT COUNT(*) FROM profiles WHERE profiles.user_id = users.id) AS profile_count,
       CAST(COALESCE((SELECT MAX(auth_sessions.last_seen_at) FROM auth_sessions
                      WHERE auth_sessions.user_id = users.id), 0) AS INTEGER) AS last_activity_at
FROM users
JOIN profiles AS primary_profile ON primary_profile.user_id = users.id AND primary_profile.is_primary = 1
ORDER BY users.name_key, users.id;

-- UpdatePrimaryProfile keeps the primary profile's name and limit in step with its account.
-- name: UpdatePrimaryProfile :exec
UPDATE profiles
SET name = ?, max_age = ?, hide_unrated = ?, updated_at = ?
WHERE user_id = ? AND is_primary = 1;

-- name: ListUserLibraryIDs :many
SELECT library_id FROM user_libraries
WHERE user_id = ?
ORDER BY library_id;

-- name: ListAllUserLibraryIDs :many
SELECT user_id, library_id FROM user_libraries
ORDER BY user_id, library_id;

-- name: DeleteUserLibraries :exec
DELETE FROM user_libraries
WHERE user_id = ?;

-- name: InsertUserLibrary :exec
INSERT INTO user_libraries (user_id, library_id)
VALUES (?, ?);

-- name: InsertProfile :exec
INSERT INTO profiles (id, user_id, name, is_primary, avatar_color, pin_hash, max_age, hide_unrated,
                      all_libraries, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateProfile :execrows
UPDATE profiles
SET name = ?, avatar_color = ?, max_age = ?, hide_unrated = ?, all_libraries = ?, updated_at = ?
WHERE id = ?;

-- name: SetProfilePINHash :execrows
UPDATE profiles
SET pin_hash = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteProfile :execrows
DELETE FROM profiles
WHERE id = ?;

-- name: GetProfile :one
SELECT id, user_id, name, is_primary, avatar_color, pin_hash, max_age, hide_unrated, all_libraries,
       created_at, updated_at
FROM profiles
WHERE id = ?;

-- name: GetPrimaryProfile :one
SELECT id, user_id, name, is_primary, avatar_color, pin_hash, max_age, hide_unrated, all_libraries,
       created_at, updated_at
FROM profiles
WHERE user_id = ? AND is_primary = 1;

-- ListProfilesOfUser returns the primary profile first, then the managed ones in creation
-- order.
-- name: ListProfilesOfUser :many
SELECT id, user_id, name, is_primary, avatar_color, pin_hash, max_age, hide_unrated, all_libraries,
       created_at, updated_at
FROM profiles
WHERE user_id = ?
ORDER BY is_primary DESC, created_at, id;

-- name: CountManagedProfiles :one
SELECT COUNT(*) FROM profiles
WHERE user_id = ? AND is_primary = 0;

-- name: ListProfileLibraryIDs :many
SELECT library_id FROM profile_libraries
WHERE profile_id = ?
ORDER BY library_id;

-- name: DeleteProfileLibraries :exec
DELETE FROM profile_libraries
WHERE profile_id = ?;

-- name: InsertProfileLibrary :exec
INSERT INTO profile_libraries (profile_id, library_id)
VALUES (?, ?);
