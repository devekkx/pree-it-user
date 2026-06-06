-- name: CreateProfile :one
INSERT INTO profiles (id, display_name, username, created_at, updated_at)
VALUES ($1, $2, $3, NOW(), NOW())
RETURNING *;

-- name: GetProfileByID :one
SELECT * FROM profiles
WHERE id = $1 AND is_active = true;

-- name: GetProfileByUsername :one
SELECT * FROM profiles
WHERE username = $1 AND is_active = true;

-- name: UpdateProfile :one
UPDATE profiles
SET
    display_name = COALESCE(sqlc.narg('display_name'), display_name),
    username     = COALESCE(sqlc.narg('username'), username),
    avatar_url   = COALESCE(sqlc.narg('avatar_url'), avatar_url),
    bio          = COALESCE(sqlc.narg('bio'), bio),
    updated_at   = NOW()
WHERE id = $1 AND is_active = true
RETURNING *;

-- name: SearchProfiles :many
SELECT * FROM profiles
WHERE is_active = true
  AND (
      display_name ILIKE '%' || @query::text || '%'
      OR username ILIKE '%' || @query::text || '%'
  )
ORDER BY
    CASE
        WHEN username = @query::text THEN 0
        WHEN username ILIKE @query::text || '%' THEN 1
        WHEN display_name ILIKE @query::text || '%' THEN 2
        ELSE 3
    END,
    created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdateLastSeen :exec
UPDATE profiles
SET last_seen_at = NOW()
WHERE id = $1 AND is_active = true;

-- name: DeactivateProfile :exec
UPDATE profiles
SET is_active = false, updated_at = NOW()
WHERE id = $1;

-- name: CheckUsernameExists :one
SELECT EXISTS(
    SELECT 1 FROM profiles WHERE username = $1
) AS exists;

-- name: GetProfilesByIDs :many
SELECT * FROM profiles
WHERE id = ANY($1::uuid[]) AND is_active = true;