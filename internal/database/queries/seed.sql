-- The seed queries add a row only if it is not already there, so seeding can run on every start

-- name: SeedTeam :exec
INSERT INTO teams (name) VALUES (?)
ON CONFLICT (name) DO NOTHING;

-- name: SeedEnvironment :exec
INSERT INTO environments (name, is_production, sort_order) VALUES (?, ?, ?)
ON CONFLICT (name) DO NOTHING;

-- name: SeedEventType :exec
INSERT INTO event_types (code, description) VALUES (?, ?)
ON CONFLICT (code) DO NOTHING;

-- name: SeedUser :exec
INSERT INTO users (email, display_name, password_hash, role, team_id) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (email) DO NOTHING;

-- name: SeedWebhook :one
INSERT INTO webhooks (team_id, environment_id, url, secret_ciphertext, secret_last4, active) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (team_id, environment_id) DO NOTHING
RETURNING id;

-- name: SeedWebhookEvent :execrows
INSERT INTO webhook_events (webhook_id, event_type_id)
SELECT sqlc.arg(webhook_id), event_types.id FROM event_types WHERE event_types.code = sqlc.arg(code);
