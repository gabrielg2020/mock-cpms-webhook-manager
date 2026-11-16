package authz

import "testing"

const (
	ownTeam   int64 = 1
	otherTeam int64 = 2
)

// One row for each webhook action in the plan's permission matrix: may a member do it
// to their own team's webhook outside production, and in production?
var matrix = []struct {
	name                string
	action              Action
	memberNonProduction bool
	memberProduction    bool
}{
	{name: "view", action: ViewWebhook, memberNonProduction: true, memberProduction: true},
	{name: "create", action: CreateWebhook, memberNonProduction: true, memberProduction: false},
	{name: "update", action: UpdateWebhook, memberNonProduction: true, memberProduction: false},
	{name: "rotate secret", action: RotateSecret, memberNonProduction: true, memberProduction: false},
	{name: "send test event", action: SendTestEvent, memberNonProduction: true, memberProduction: true},
	{name: "delete", action: DeleteWebhook, memberNonProduction: false, memberProduction: false},
}

func TestAllowedMatchesThePermissionMatrix(t *testing.T) {
	checked := 0
	for _, row := range matrix {
		for _, production := range []bool{false, true} {
			target := WebhookTarget{TeamID: ownTeam, IsProduction: production}
			memberMay := row.memberNonProduction
			if production {
				memberMay = row.memberProduction
			}

			testCases := []struct {
				who     string
				subject Subject
				want    bool
			}{
				{who: "admin with no team", subject: Subject{UserID: 1, Role: RoleAdmin}, want: true},
				{who: "admin in another team", subject: Subject{UserID: 1, Role: RoleAdmin, TeamID: otherTeam}, want: true},
				{who: "member of the team", subject: Subject{UserID: 2, Role: RoleMember, TeamID: ownTeam}, want: memberMay},
				{who: "member of another team", subject: Subject{UserID: 3, Role: RoleMember, TeamID: otherTeam}, want: false},
				{who: "unknown role in the team", subject: Subject{UserID: 4, Role: "auditor", TeamID: ownTeam}, want: false},
				{who: "no role in the team", subject: Subject{UserID: 5, TeamID: ownTeam}, want: false},
			}
			for _, testCase := range testCases {
				checked++
				if got := Allowed(testCase.subject, row.action, target); got != testCase.want {
					t.Errorf("%s, %s, production %t: allowed = %t, want %t", testCase.who, row.name, production, got, testCase.want)
				}
			}
		}
	}
	if checked != 72 {
		t.Errorf("combinations checked = %d, want 72", checked)
	}
}

func TestAMemberWithNoTeamMayDoNothing(t *testing.T) {
	member := Subject{UserID: 2, Role: RoleMember}

	// A webhook with no team cannot exist, but if one were asked about, 0 must not match 0.
	for _, row := range matrix {
		if Allowed(member, row.action, WebhookTarget{}) {
			t.Errorf("a member with no team may %s a webhook with no team", row.name)
		}
	}
}

func TestAMemberMayNotDoAnActionThatIsNotListed(t *testing.T) {
	member := Subject{UserID: 2, Role: RoleMember, TeamID: ownTeam}

	if Allowed(member, Action(99), WebhookTarget{TeamID: ownTeam}) {
		t.Error("a member may do an action the matrix does not list")
	}
}

func TestIsAdmin(t *testing.T) {
	testCases := []struct {
		role Role
		want bool
	}{
		{role: RoleAdmin, want: true},
		{role: RoleMember, want: false},
		{role: "Admin", want: false},
		{role: "", want: false},
	}

	for _, testCase := range testCases {
		if got := (Subject{Role: testCase.role}).IsAdmin(); got != testCase.want {
			t.Errorf("role %q: is admin = %t, want %t", testCase.role, got, testCase.want)
		}
	}
}

func TestVisibleTeam(t *testing.T) {
	testCases := []struct {
		name          string
		subject       Subject
		wantTeam      int64
		wantEveryTeam bool
	}{
		{name: "an admin sees every team", subject: Subject{Role: RoleAdmin}, wantEveryTeam: true},
		{name: "an admin in a team still sees every team", subject: Subject{Role: RoleAdmin, TeamID: ownTeam}, wantEveryTeam: true},
		{name: "a member sees their own team", subject: Subject{Role: RoleMember, TeamID: ownTeam}, wantTeam: ownTeam},
		{name: "a member with no team sees no team", subject: Subject{Role: RoleMember}},
		{name: "an unknown role sees no team, even when it has one", subject: Subject{Role: "auditor", TeamID: ownTeam}},
		{name: "no role sees no team", subject: Subject{TeamID: ownTeam}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			team, everyTeam := VisibleTeam(testCase.subject)

			if team != testCase.wantTeam || everyTeam != testCase.wantEveryTeam {
				t.Errorf("got team %d and every team %t, want team %d and every team %t", team, everyTeam, testCase.wantTeam, testCase.wantEveryTeam)
			}
		})
	}
}
