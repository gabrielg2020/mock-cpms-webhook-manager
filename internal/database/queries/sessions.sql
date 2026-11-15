-- name: GetSession :one
SELECT
  data
FROM
  sessions
WHERE
  token = ?
  AND expiry > ?;
-- name: SaveSession :exec
INSERT INTO sessions(
  token,
  data,
  expiry
)
VALUES(
  ?,
  ?,
  ?
)
ON CONFLICT (token) DO UPDATE 
SET data = excluded.data,
expiry = excluded.expiry;
-- name: DeleteSession :exec
DELETE FROM sessions
WHERE
  token = ?;
-- name: DeleteExpiredSessions :exec
DELETE FROM sessions
WHERE
  expiry <= ?;
