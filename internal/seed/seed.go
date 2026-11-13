package seed

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/auth"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/store"
)

const (
	AdminEmail  = "admin@example.com"
	MemberEmail = "member@example.com"
	memberTeam  = "MOT"
)

type Passwords struct {
	Admin  string
	Member string
}

var teams = []string{memberTeam, "Theory Test"}

var environments = []store.SeedEnvironmentParams{
	{Name: "dev01", IsProduction: 0, SortOrder: 1},
	{Name: "int", IsProduction: 0, SortOrder: 2},
	{Name: "preprod", IsProduction: 0, SortOrder: 3},
	{Name: "prod", IsProduction: 1, SortOrder: 4},
}

var eventTypes = []store.SeedEventTypeParams{
	{Code: "payment.succeeded", Description: "A payment was taken"},
	{Code: "payment.failed", Description: "A payment was declined or abandoned"},
	{Code: "refund.succeeded", Description: "A refund was paid"},
	{Code: "refund.failed", Description: "A refund could not be paid"},
}

// DemoData adds whatever demo data is missing and leaves existing rows alone, therefore
// safe to run on every start. It runs one transaction.
func DemoData(ctx context.Context, db *sql.DB, passwords Passwords) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	queries := store.New(tx)

	for _, team := range teams {
		if err := queries.SeedTeam(ctx, team); err != nil {
			return fmt.Errorf("seed team %s: %w", team, err)
		}
	}
	for _, environment := range environments {
		if err := queries.SeedEnvironment(ctx, environment); err != nil {
			return fmt.Errorf("seed environment %s: %w", environment.Name, err)
		}
	}
	for _, eventType := range eventTypes {
		if err := queries.SeedEventType(ctx, eventType); err != nil {
			return fmt.Errorf("seed event type %s: %w", eventType.Code, err)
		}
	}

	team, err := queries.GetTeamByName(ctx, memberTeam)
	if err != nil {
		return fmt.Errorf("find team %s: %w", memberTeam, err)
	}
	users := []store.SeedUserParams{
		{
			Email:        AdminEmail,
			DisplayName:  "Demo Admin",
			PasswordHash: auth.HashPassword(passwords.Admin),
			Role:         "admin",
		},
		{
			Email:        MemberEmail,
			DisplayName:  "Demo Member",
			PasswordHash: auth.HashPassword(passwords.Member),
			Role:         "member",
			TeamID:       sql.NullInt64{Int64: team.ID, Valid: true},
		},
	}
	for _, user := range users {
		if err := queries.SeedUser(ctx, user); err != nil {
			return fmt.Errorf("seed user %s: %w", user.Email, err)
		}
	}

	return tx.Commit()
}
