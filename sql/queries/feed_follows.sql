-- name: CreateFeedFollow :one
INSERT INTO feed_follows (id, created_at, updated_at, user_id, feed_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetFeedFollowForUser :many
SELECT * FROM feed_follows where user_id = $1;

-- name: DeletFeedFollow :exec
DELETE FROM feed_follows
  WHERE id = $1;


