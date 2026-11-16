package seed

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/auth"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/secretbox"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/store"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/webhook"
)

const (
	AdminEmail  = "admin@example.com"
	MemberEmail = "member@example.com"
	memberTeam  = "MOT"
	otherTeam   = "Theory Test"
)

var teams = []string{memberTeam, otherTeam}

type Passwords struct {
	Admin  string
	Member string
}

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

type demoWebhook struct {
	team        string
	environment string
	url         string
	active      int64
	events      []string
}

var allEvents = []string{"payment.succeeded", "payment.failed", "refund.succeeded", "refund.failed"}

// The member's team has no webhook for int, so there is somewhere left to create one.
// example.com is reserved for docs
var demoWebhooks = []demoWebhook{
	{team: memberTeam, environment: "dev01", url: "https://mot-dev01.example.com/cpms/webhooks", active: 1, events: allEvents},
	{team: memberTeam, environment: "preprod", url: "https://mot-preprod.example.com/cpms/webhooks", active: 0, events: []string{"payment.succeeded", "payment.failed"}},
	{team: memberTeam, environment: "prod", url: "https://mot.example.com/cpms/webhooks", active: 1, events: []string{"payment.succeeded", "refund.succeeded"}},
	{team: otherTeam, environment: "dev01", url: "https://theory-test-dev01.example.com/cpms/webhooks", active: 1, events: []string{"payment.succeeded"}},
	{team: otherTeam, environment: "prod", url: "https://theory-test.example.com/cpms/webhooks", active: 1, events: allEvents},
}

func seedWebhook(ctx context.Context, queries *store.Queries, secrets *secretbox.Box, demo demoWebhook) error {
	team, err := queries.GetTeamByName(ctx, demo.team)
	if err != nil {
		return fmt.Errorf("find team: %w", err)
	}
	environment, err := queries.GetEnvironmentByName(ctx, demo.environment)
	if err != nil {
		return fmt.Errorf("find environment: %w", err)
	}

	// Nobody ever sees a seeded secret. A team gets one it knows by rotating it.
	secret, lastFour := webhook.NewSecret()
	id, err := queries.SeedWebhook(ctx, store.SeedWebhookParams{
		TeamID:           team.ID,
		EnvironmentID:    environment.ID,
		URL:              demo.url,
		SecretCiphertext: secrets.Seal([]byte(secret)),
		SecretLast4:      lastFour,
		Active:           demo.active,
	})
	if errors.Is(err, sql.ErrNoRows) {
		// The webhook is already there, so it and its events are left as they are
		return nil
	}
	if err != nil {
		return err
	}

	for _, code := range demo.events {
		added, err := queries.SeedWebhookEvent(ctx, store.SeedWebhookEventParams{WebhookID: id, Code: code})
		if err != nil {
			return fmt.Errorf("subscribe to %s: %w", code, err)
		}
		if added != 1 {
			return fmt.Errorf("subscribe to %s: no such event type", code)
		}
	}
	return nil
}

// DemoData adds whatever demo data is missing and leaves existing rows alone, therefore
// safe to run on every start. It runs one transaction.
func DemoData(ctx context.Context, db *sql.DB, passwords Passwords, secrets *secretbox.Box) error {
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

	for _, demo := range demoWebhooks {
		if err := seedWebhook(ctx, queries, secrets, demo); err != nil {
			return fmt.Errorf("seed webhook for %s %s: %w", demo.team, demo.environment, err)
		}
	}

	return tx.Commit()
}
