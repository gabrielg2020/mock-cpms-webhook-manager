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
