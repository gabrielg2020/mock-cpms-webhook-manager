-- name: ListWebhooks :many
SELECT
  webhooks.id,
  webhooks.team_id,
  teams.name AS team_name,
  environments.name AS environment_name,
  environments.is_production,
  webhooks.url,
  webhooks.active
FROM
  webhooks
JOIN teams
  ON teams.id = webhooks.team_id
JOIN environments
  ON environments.id = webhooks.environment_id
ORDER BY
  teams.name,
  environments.sort_order;
-- name: ListWebhooksForTeam :many
SELECT
  webhooks.id,
  webhooks.team_id,
  teams.name AS team_name,
  environments.name AS environment_name,
  environments.is_production,
  webhooks.url,
  webhooks.active
FROM
  webhooks
JOIN teams
  ON teams.id = webhooks.team_id
JOIN environments
  ON environments.id = webhooks.environment_id
WHERE
  webhooks.team_id = ?
ORDER BY
  teams.name,
  environments.sort_order;
-- name: GetWebhook :one
SELECT
  webhooks.id,
  webhooks.team_id,
  teams.name AS team_name,
  environments.name AS environment_name,
  environments.is_production,
  webhooks.url,
  webhooks.active,
  webhooks.secret_last4,
  webhooks.updated_at
FROM
  webhooks
JOIN teams
  ON teams.id = webhooks.team_id
JOIN environments
  ON environments.id = webhooks.environment_id
WHERE
  webhooks.id = ?;
-- name: GetWebhookForTeam :one
SELECT
  webhooks.id,
  webhooks.team_id,
  teams.name AS team_name,
  environments.name AS environment_name,
  environments.is_production,
  webhooks.url,
  webhooks.active,
  webhooks.secret_last4,
  webhooks.updated_at
FROM
  webhooks
JOIN teams
  ON teams.id = webhooks.team_id
JOIN environments
  ON environments.id = webhooks.environment_id
WHERE
  webhooks.id = ?
  AND webhooks.team_id = ?;
-- name: ListWebhookEventTypes :many
SELECT
  event_types.code,
  event_types.description
FROM
  webhook_events
JOIN event_types
  ON event_types.id = webhook_events.event_type_id
WHERE
  webhook_events.webhook_id = ?
ORDER BY
  event_types.code;
