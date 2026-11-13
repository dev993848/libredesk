package main

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	amodels "github.com/abhinavxd/libredesk/internal/auth/models"
	"github.com/abhinavxd/libredesk/internal/authz"
	cmodels "github.com/abhinavxd/libredesk/internal/conversation/models"
	"github.com/abhinavxd/libredesk/internal/envelope"
	"github.com/abhinavxd/libredesk/internal/timetracking"
	umodels "github.com/abhinavxd/libredesk/internal/user/models"
	"github.com/zerodha/fastglue"
)

// timeTrackingContext loads the current agent and the conversation and checks read access.
func timeTrackingContext(r *fastglue.Request, uuid string) (umodels.User, cmodels.Conversation, error) {
	app := r.Context.(*App)
	auser := r.RequestCtx.UserValue("user").(amodels.User)

	user, err := app.user.GetAgentCachedOrLoad(auser.ID)
	if err != nil {
		return user, cmodels.Conversation{}, err
	}

	conv, err := app.conversation.GetConversation(0, uuid, "")
	if err != nil {
		return user, conv, err
	}

	ok, err := app.authz.EnforceConversationAccess(user, conv)
	if err != nil {
		return user, conv, err
	}
	if !ok {
		return user, conv, envelope.NewError(envelope.PermissionError, app.i18n.T("status.deniedPermission"), nil)
	}
	return user, conv, nil
}

// handleTimeStart starts the agent's timer on a conversation.
func handleTimeStart(r *fastglue.Request) error {
	app := r.Context.(*App)
	uuid := r.RequestCtx.UserValue("uuid").(string)

	user, _, err := timeTrackingContext(r, uuid)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if ok, _ := app.authz.Enforce(user, "conversations", "write"); !ok {
		return r.SendErrorEnvelope(http.StatusForbidden, app.i18n.T("status.deniedPermission"), nil, envelope.PermissionError)
	}

	entry, err := app.timeTracking.Start(uuid, user.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(entry)
}

// handleTimeStop stops the agent's running timer.
func handleTimeStop(r *fastglue.Request) error {
	app := r.Context.(*App)
	uuid := r.RequestCtx.UserValue("uuid").(string)

	user, _, err := timeTrackingContext(r, uuid)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if ok, _ := app.authz.Enforce(user, "conversations", "write"); !ok {
		return r.SendErrorEnvelope(http.StatusForbidden, app.i18n.T("status.deniedPermission"), nil, envelope.PermissionError)
	}

	entry, err := app.timeTracking.Stop(user.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(entry)
}

// handleTimeList returns the entries, totals and the agent's active timer for a conversation.
func handleTimeList(r *fastglue.Request) error {
	app := r.Context.(*App)
	uuid := r.RequestCtx.UserValue("uuid").(string)

	user, _, err := timeTrackingContext(r, uuid)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	entries, err := app.timeTracking.List(uuid)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	totals, err := app.timeTracking.Totals(uuid)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	active, hasActive, err := app.timeTracking.Active(user.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	var activeValue any
	if hasActive {
		activeValue = active
	}
	return r.SendEnvelope(map[string]any{
		"entries":          entries,
		"total_seconds":    totals.TotalSeconds,
		"billable_seconds": totals.BillableSeconds,
		"active":           activeValue,
		"has_active":       hasActive,
	})
}

// handleTimeAdd records a manual time entry.
func handleTimeAdd(r *fastglue.Request) error {
	app := r.Context.(*App)
	uuid := r.RequestCtx.UserValue("uuid").(string)

	user, _, err := timeTrackingContext(r, uuid)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if ok, _ := app.authz.Enforce(user, "conversations", "write"); !ok {
		return r.SendErrorEnvelope(http.StatusForbidden, app.i18n.T("status.deniedPermission"), nil, envelope.PermissionError)
	}

	var req struct {
		DurationSeconds int    `json:"duration_seconds"`
		Note            string `json:"note"`
		Billable        *bool  `json:"billable"`
		StartedAt       string `json:"started_at"`
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(http.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}

	billable := true
	if req.Billable != nil {
		billable = *req.Billable
	}
	startedAt := time.Time{}
	if strings.TrimSpace(req.StartedAt) != "" {
		startedAt, err = time.Parse(time.RFC3339, req.StartedAt)
		if err != nil {
			return r.SendErrorEnvelope(http.StatusBadRequest, app.i18n.T("validation.invalidTimeFormat"), nil, envelope.InputError)
		}
	}

	entry, err := app.timeTracking.AddManual(uuid, user.ID, req.DurationSeconds, strings.TrimSpace(req.Note), billable, startedAt)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(entry)
}

// handleTimeUpdate edits an existing entry.
func handleTimeUpdate(r *fastglue.Request) error {
	app := r.Context.(*App)
	auser := r.RequestCtx.UserValue("user").(amodels.User)

	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id <= 0 {
		return r.SendErrorEnvelope(http.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}

	entry, err := app.timeTracking.Get(id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := canManageEntry(r, auser.ID, entry); err != nil {
		return sendErrorEnvelope(r, err)
	}

	var req struct {
		DurationSeconds int    `json:"duration_seconds"`
		Note            string `json:"note"`
		Billable        *bool  `json:"billable"`
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(http.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}
	billable := entry.Billable
	if req.Billable != nil {
		billable = *req.Billable
	}

	updated, err := app.timeTracking.Update(id, req.DurationSeconds, strings.TrimSpace(req.Note), billable)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(updated)
}

// handleTimeDelete removes an entry.
func handleTimeDelete(r *fastglue.Request) error {
	app := r.Context.(*App)
	auser := r.RequestCtx.UserValue("user").(amodels.User)

	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id <= 0 {
		return r.SendErrorEnvelope(http.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}

	entry, err := app.timeTracking.Get(id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := canManageEntry(r, auser.ID, entry); err != nil {
		return sendErrorEnvelope(r, err)
	}

	if _, err := app.timeTracking.Delete(id); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(map[string]any{"deleted": true})
}

// canManageTimeEntry reports whether the actor may edit or delete a time entry.
// The entry author is always allowed. Other agents need time_tracking:manage and
// read access to the entry's conversation, which keeps team-scoped leads inside
// their own teams.
func canManageTimeEntry(user umodels.User, entry timetracking.Entry, conv cmodels.Conversation) bool {
	if entry.UserID.Valid && user.ID == entry.UserID.Int {
		return true
	}
	if !slices.Contains(user.Permissions, "time_tracking:manage") {
		return false
	}
	return authz.CanReadAssignment(user, conv.AssignedUserID, conv.AssignedTeamID)
}

// canManageEntry allows the entry author and holders of time_tracking:manage.
// Managers are additionally restricted to conversations they can read, so a team
// lead with team-scoped read permissions can only manage entries within their teams.
func canManageEntry(r *fastglue.Request, actorID int, entry timetracking.Entry) error {
	app := r.Context.(*App)
	if entry.UserID.Valid && actorID == entry.UserID.Int {
		return nil
	}
	user, err := app.user.GetAgentCachedOrLoad(actorID)
	if err != nil {
		return err
	}
	conv, err := app.conversation.GetConversation(entry.ConversationID, "", "")
	if err != nil {
		return err
	}
	if !canManageTimeEntry(user, entry, conv) {
		return envelope.NewError(envelope.PermissionError, app.i18n.T("status.deniedPermission"), nil)
	}
	return nil
}
