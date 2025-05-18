-- name: CreateEvent :one
INSERT INTO events (
    name, 
    description,
    date
) VALUES (
    sqlc.arg(name),
    sqlc.arg(description),
    sqlc.arg(date)
)
RETURNING *;
-- name: UpdateEvent :one
UPDATE events
SET name = sqlc.arg(name),
    description = sqlc.arg(description),
    date = COALESCE(sqlc.arg(date), date)
WHERE id = sqlc.arg(id)
RETURNING *;
-- name: GetEvents :many    
SELECT * FROM events ORDER BY created_at DESC;
-- name: DeleteEvent :exec
DELETE FROM events
WHERE id = sqlc.arg(id);
-- name: GetEventByID :one  
SELECT * FROM events
WHERE id = sqlc.arg(id);
-- name: GetLastEvent :one
SELECT * FROM events
WHERE id = (
    SELECT id FROM events
    ORDER BY created_at DESC
    LIMIT 1
);
