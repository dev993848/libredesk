package main

import (
	"testing"

	cmodels "github.com/abhinavxd/libredesk/internal/conversation/models"
	tmodels "github.com/abhinavxd/libredesk/internal/team/models"
	"github.com/abhinavxd/libredesk/internal/timetracking"
	umodels "github.com/abhinavxd/libredesk/internal/user/models"
	"github.com/volatiletech/null/v9"
)

func TestCanManageTimeEntry(t *testing.T) {
	const (
		author    = 1
		lead      = 2
		team      = 10
		otherTeam = 11
	)

	entry := timetracking.Entry{UserID: null.IntFrom(author), ConversationID: 1}
	teamConversation := cmodels.Conversation{AssignedTeamID: null.IntFrom(team)}
	otherTeamConversation := cmodels.Conversation{AssignedTeamID: null.IntFrom(otherTeam)}
	unassignedConversation := cmodels.Conversation{}

	user := func(id int, perms []string, teams ...int) umodels.User {
		compact := make(tmodels.TeamsCompact, 0, len(teams))
		for _, teamID := range teams {
			compact = append(compact, tmodels.TeamCompact{ID: teamID})
		}
		return umodels.User{ID: id, Permissions: perms, Teams: compact}
	}

	tests := []struct {
		name string
		user umodels.User
		conv cmodels.Conversation
		want bool
	}{
		{
			name: "author edits their own entry",
			user: user(author, nil),
			conv: otherTeamConversation,
			want: true,
		},
		{
			name: "manager with read_all edits any entry",
			user: user(lead, []string{"conversations:read", "conversations:read_all", "time_tracking:manage"}, team),
			conv: otherTeamConversation,
			want: true,
		},
		{
			name: "team lead edits an entry of their team",
			user: user(lead, []string{"conversations:read", "conversations:read_team_all", "time_tracking:manage"}, team),
			conv: teamConversation,
			want: true,
		},
		{
			name: "team lead cannot edit another team's entry",
			user: user(lead, []string{"conversations:read", "conversations:read_team_all", "time_tracking:manage"}, team),
			conv: otherTeamConversation,
			want: false,
		},
		{
			name: "manage permission without conversation read is denied",
			user: user(lead, []string{"time_tracking:manage"}, team),
			conv: teamConversation,
			want: false,
		},
		{
			name: "conversation read without manage permission is denied",
			user: user(lead, []string{"conversations:read", "conversations:read_team_all"}, team),
			conv: teamConversation,
			want: false,
		},
		{
			name: "unassigned entry needs unassigned or read_all scope",
			user: user(lead, []string{"conversations:read", "conversations:read_team_all", "time_tracking:manage"}, team),
			conv: unassignedConversation,
			want: false,
		},
		{
			name: "unassigned scope allows unassigned entry",
			user: user(lead, []string{"conversations:read", "conversations:read_unassigned", "time_tracking:manage"}),
			conv: unassignedConversation,
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := canManageTimeEntry(tc.user, entry, tc.conv); got != tc.want {
				t.Fatalf("canManageTimeEntry() = %v, want %v", got, tc.want)
			}
		})
	}
}
