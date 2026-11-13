-- name: GetTeamByName :one
SELECT
  *
FROM
  teams
WHERE
  name = ?;
