package authz

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

type Action int

const (
	ViewWebhook Action = iota
	CreateWebhook
	UpdateWebhook
	RotateSecret
	SendTestEvent
	DeleteWebhook
)

type Subject struct {
	UserID int64
	Role   Role
	TeamID int64
}

type WebhookTarget struct {
	TeamID       int64
	IsProduction bool
}

func (s Subject) IsAdmin() bool {
	return s.Role == RoleAdmin
}

// Allowed is the permission matrix. Admins can do anything. Members may do only
// whats defined here, and only to their team's webhooks.
func Allowed(subject Subject, action Action, target WebhookTarget) bool {
	if subject.Role == RoleAdmin {
		return true
	}
	if subject.Role != RoleMember || subject.TeamID == 0 || subject.TeamID != target.TeamID {
		return false
	}

	switch action {
	case ViewWebhook, SendTestEvent:
		return true
	case CreateWebhook, UpdateWebhook, RotateSecret:
		return !target.IsProduction
	default:
		return false
	}
}
