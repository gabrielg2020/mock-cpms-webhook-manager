-- name: GetUserByEmail :one
SELECT
  *
FROM
  users
WHERE
  email = ?;
-- name: GetSignedInUser :one
SELECT
  users.id,
  users.display_name,
  users.role,
  users.team_id,
  teams.name AS team_name
FROM
  users
  LEFT JOIN teams ON teams.id = users.team_id
WHERE
  users.id = ?;
-- name: CountFailedSignIn :one
UPDATE users
SET
  failed_logins = failed_logins + 1
WHERE
  id = ?
RETURNING
  failed_logins;
-- name: LockUser :exec
UPDATE users
SET
  locked_until = ?,
  failed_logins = 0
WHERE
  id = ?;
-- name: ClearFailedSignIns :exec
UPDATE users
SET
  failed_logins = 0,
  locked_until = NULL
WHERE
  id = ?;
