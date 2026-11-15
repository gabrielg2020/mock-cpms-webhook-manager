-- name: GetUserByEmail :one
SELECT
  *
FROM
  users
WHERE
  email = ?;
-- name: GetSignedInUser :one
SELECT
  id,
  display_name,
  role,
  team_id
FROM
  users
WHERE
  id = ?;
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
