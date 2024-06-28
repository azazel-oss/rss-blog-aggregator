-- name: CreatePost :one
INSERT INTO posts (id, created_at, updated_at, title, url, description, feed_id, published_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetPostsByUser :many
SELECT p.* FROM posts p
JOIN feeds f
ON f.id = p.feed_id
WHERE f.user_id = $1
LIMIT $2;
