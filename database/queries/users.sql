-- name: CreateUser :one
INSERT INTO users (
    name, 
    username,
    tg_id,
    event_id
) VALUES (
    sqlc.arg(name),
    sqlc.arg(username),
    sqlc.arg(tg_id),
    sqlc.arg(event_id)
) RETURNING *;
-- name: DeleteUser :exec
DELETE FROM users
WHERE id = sqlc.arg(id);
-- name: GetUserByID :one
SELECT * FROM users
WHERE id = sqlc.arg(id);
-- name: GetUserByUsername :one
SELECT * FROM users
WHERE username = sqlc.arg(username);
-- name: GetUsersByEventID :many
SELECT * FROM users
WHERE event_id = sqlc.arg(event_id);
-- name: DeleteUsersByIdAndEventId :exec
DELETE FROM users
WHERE id = sqlc.arg(id) AND event_id = sqlc.arg(event_id);
-- name: UpdateUserN :exec
UPDATE users
SET n = sqlc.arg(n)
WHERE id = sqlc.arg(id)
AND event_id = sqlc.arg(event_id);
