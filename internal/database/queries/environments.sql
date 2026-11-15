-- name: GetEnvironmentByName :one
SELECT
  *
FROM
  environments
WHERE
  name = ?;
