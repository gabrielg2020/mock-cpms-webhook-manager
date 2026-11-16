package web

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/authz"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/store"
	"github.com/go-chi/chi/v5"
)

// errWebhookNotFound covers a webhook that does not exist and one the user can't see,
// so that nothing downstream can tell the two apart.
var errWebhookNotFound = errors.New("webhook not found")

type webhookSummary struct {
	ID           int64
	Team         string
	Environment  string
	IsProduction bool
	URL          string
	Active       bool
}

type webhookListPage struct {
	Webhooks []webhookSummary
	ShowTeam bool
}

type webhookDetailPage struct {
	Webhook        webhookSummary
	Events         []store.ListWebhookEventTypesRow
	SecretLastFour string
	UpdatedOn      string
}

func (app *Application) webhookList(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	teamID, everyTeam := authz.VisibleTeam(user.Subject)

	var rows []store.ListWebhooksRow
	var err error
	if everyTeam {
		rows, err = app.queries.ListWebhooks(r.Context())
	} else {
		var teamRows []store.ListWebhooksForTeamRow
		teamRows, err = app.queries.ListWebhooksForTeam(r.Context(), teamID)
		for _, row := range teamRows {
			rows = append(rows, store.ListWebhooksRow(row))
		}
	}
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	page := webhookListPage{ShowTeam: everyTeam}
	for _, row := range rows {
		if !app.mayView(r.Context(), user, row.ID, row.TeamID, row.IsProduction) {
			continue
		}
		page.Webhooks = append(page.Webhooks, webhookSummary{
			ID:           row.ID,
			Team:         row.TeamName,
			Environment:  row.EnvironmentName,
			IsProduction: row.IsProduction == 1,
			URL:          row.URL,
			Active:       row.Active == 1,
		})
	}
	app.render(w, r, http.StatusOK, "webhooks", page)
}

func (app *Application) webhookDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := webhookID(r)
	if !ok {
		app.notFound(w, r)
		return
	}

	user, _ := userFrom(r.Context())
	row, err := app.findWebhook(r.Context(), user, id)
	if errors.Is(err, errWebhookNotFound) {
		app.notFound(w, r)
		return
	}
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	events, err := app.queries.ListWebhookEventTypes(r.Context(), row.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.render(w, r, http.StatusOK, "webhook", webhookDetailPage{
		Webhook: webhookSummary{
			ID:           row.ID,
			Team:         row.TeamName,
			Environment:  row.EnvironmentName,
			IsProduction: row.IsProduction == 1,
			URL:          row.URL,
			Active:       row.Active == 1,
		},
		Events:         events,
		SecretLastFour: row.SecretLast4,
		UpdatedOn:      displayDate(row.UpdatedAt),
	})
}

// findWebhook runs the query for what this user may see, then asks the policy as well.
func (app *Application) findWebhook(ctx context.Context, user currentUser, id int64) (store.GetWebhookRow, error) {
	teamID, everyTeam := authz.VisibleTeam(user.Subject)

	var row store.GetWebhookRow
	var err error
	if everyTeam {
		row, err = app.queries.GetWebhook(ctx, id)
	} else {
		var teamRow store.GetWebhookForTeamRow
		teamRow, err = app.queries.GetWebhookForTeam(ctx, store.GetWebhookForTeamParams{ID: id, TeamID: teamID})
		row = store.GetWebhookRow(teamRow)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return store.GetWebhookRow{}, errWebhookNotFound
	}
	if err != nil {
		return store.GetWebhookRow{}, err
	}

	if !app.mayView(ctx, user, row.ID, row.TeamID, row.IsProduction) {
		return store.GetWebhookRow{}, errWebhookNotFound
	}
	return row, nil
}

// mayView is the second check on every webhook a query returns. The query has already
// filtered by team, so a refusal here means the query is wrong, and that is logged as an error.
func (app *Application) mayView(ctx context.Context, user currentUser, webhookID, teamID, isProduction int64) bool {
	target := authz.WebhookTarget{TeamID: teamID, IsProduction: isProduction == 1}
	if authz.Allowed(user.Subject, authz.ViewWebhook, target) {
		return true
	}
	app.logger.Error("a query returned a webhook the policy refuses",
		"request_id", requestIDFrom(ctx),
		"user_id", user.ID,
		"webhook_id", webhookID,
	)
	return false
}

// webhookID reads the ID from the address. Only a whole number above zero written the
// plain way is accepted, so each webhook has one address: not +1, 01 or 1.0.
func webhookID(r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != raw {
		return 0, false
	}
	return id, true
}

// displayDate turns a stored timestamp into a date such as 15 November 2026.
func displayDate(stored string) string {
	parsed, err := time.Parse(time.RFC3339, stored)
	if err != nil {
		return stored
	}
	return parsed.Format("2 January 2006")
}
