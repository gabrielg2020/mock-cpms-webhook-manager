-- name: GetUserByEmail :one
SELECT
  *
FROM
  users
WHERE
  email = ?;

-- name: GetSignedInUser :one
SELECT id, display_name, role, team_id FROM users WHERE id = ?;
