package main

import (
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"

	"github.com/abhinavxd/libredesk/internal/envelope"
	"github.com/abhinavxd/libredesk/internal/httputil"
	notifier "github.com/abhinavxd/libredesk/internal/notification"
	pmodels "github.com/abhinavxd/libredesk/internal/portal/models"
	umodels "github.com/abhinavxd/libredesk/internal/user/models"
	"github.com/valyala/fasthttp"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/fastglue"
)

const (
	portalContactCtxKey = "portal_contact"
	portalDefaultPage   = 20
	portalMaxPageSize   = 100
)

// portalAuth authenticates a customer portal session and enforces CSRF on writes.
func portalAuth(handler fastglue.FastRequestHandler) fastglue.FastRequestHandler {
	return func(r *fastglue.Request) error {
		app := r.Context.(*App)

		if !app.portal.Enabled() {
			return r.SendErrorEnvelope(http.StatusNotFound, app.i18n.T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
		}

		method := string(r.RequestCtx.Method())
		if method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete {
			cookieToken := string(r.RequestCtx.Request.Header.Cookie("csrf_token"))
			hdrToken := string(r.RequestCtx.Request.Header.Peek("X-CSRFTOKEN"))
			if cookieToken == "" || hdrToken == "" || cookieToken != hdrToken {
				return r.SendErrorEnvelope(http.StatusForbidden, app.i18n.T("auth.csrfTokenMismatch"), nil, envelope.PermissionError)
			}
		}

		sessUser, err := app.auth.ValidateContactSession(r)
		if err != nil || sessUser.ID <= 0 {
			return r.SendErrorEnvelope(http.StatusUnauthorized, app.i18n.T("auth.invalidOrExpiredSession"), nil, envelope.PermissionError)
		}

		contact, err := app.portal.GetContactByID(sessUser.ID)
		if err != nil {
			if derr := app.auth.DestroyContactSession(r); derr != nil {
				app.lo.Error("error destroying portal session", "error", derr)
			}
			return r.SendErrorEnvelope(http.StatusUnauthorized, app.i18n.T("auth.invalidOrExpiredSession"), nil, envelope.PermissionError)
		}

		r.RequestCtx.SetUserValue(portalContactCtxKey, contact)
		return handler(r)
	}
}

// getPortalContact returns the contact set by portalAuth.
func getPortalContact(r *fastglue.Request) (pmodels.Contact, error) {
	c, ok := r.RequestCtx.UserValue(portalContactCtxKey).(pmodels.Contact)
	if !ok || c.ID <= 0 {
		return pmodels.Contact{}, envelope.NewError(envelope.PermissionError, "missing portal contact", nil)
	}
	return c, nil
}

// handlePortalMagicLink sends a single-use login link to an existing contact.
func handlePortalMagicLink(r *fastglue.Request) error {
	app := r.Context.(*App)

	if !app.portal.Enabled() {
		return r.SendErrorEnvelope(http.StatusNotFound, app.i18n.T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}

	var req struct {
		Email string `json:"email"`
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(http.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" {
		return r.SendErrorEnvelope(http.StatusBadRequest, app.i18n.T("portal.auth.emailRequired"), nil, envelope.InputError)
	}
	if _, err := mail.ParseAddress(email); err != nil {
		// Do not reveal whether the address exists; still return success.
		return r.SendEnvelope(map[string]any{"sent": true})
	}

	contact, err := app.portal.GetContactByEmail(email)
	if err != nil || contact.ID <= 0 {
		// Unknown contacts get the same response to prevent enumeration.
		return r.SendEnvelope(map[string]any{"sent": true})
	}

	token, err := app.portal.CreateLoginToken(contact.ID, clientIP(r))
	if err != nil {
		app.lo.Error("error creating portal login token", "contact_id", contact.ID, "error", err)
		return sendErrorEnvelope(r, err)
	}

	link := portalBaseURL(r, app) + "/api/v1/portal/auth/verify?token=" + url.QueryEscape(token)
	go app.portal.CleanupExpiredTokens()
	go sendPortalMagicLink(app, contact, link)

	if ko.String("app.env") == "dev" {
		app.lo.Debug("portal magic link generated", "contact_id", contact.ID, "link", link)
	}
	app.lo.Info("portal magic link sent", "contact_id", contact.ID)
	return r.SendEnvelope(map[string]any{"sent": true})
}

// sendPortalMagicLink emails the login link, preferring the configured support inbox.
func sendPortalMagicLink(app *App, contact pmodels.Contact, link string) {
	subject := app.i18n.T("portal.email.subject")
	body := app.i18n.Ts("portal.email.body", "link", link)

	if inboxID := app.portal.Config().DefaultInboxID; inboxID > 0 {
		if err := app.conversation.SendTransientEmail(inboxID, 0, "", []string{contact.Email}, subject, body); err == nil {
			return
		} else {
			app.lo.Warn("error sending portal magic link via inbox, falling back to notification email", "inbox_id", inboxID, "error", err)
		}
	}

	if err := app.notifier.Send(notifier.Message{
		Provider:        notifier.ProviderEmail,
		RecipientEmails: []string{contact.Email},
		Subject:         subject,
		Content:         body,
		ContentType:     "html",
	}); err != nil {
		app.lo.Error("error queueing portal magic link email", "contact_id", contact.ID, "error", err)
	}
}

// handlePortalVerify burns a login token, starts a portal session and redirects to the portal.
func handlePortalVerify(r *fastglue.Request) error {
	app := r.Context.(*App)
	if !app.portal.Enabled() {
		return r.RedirectURI("/portal/login?error=disabled", fasthttp.StatusFound, nil, "")
	}

	token := string(r.RequestCtx.QueryArgs().Peek("token"))
	contact, err := app.portal.ConsumeLoginToken(token)
	if err != nil {
		return r.RedirectURI("/portal/login?error=invalid", fasthttp.StatusFound, nil, "")
	}

	if err := app.auth.SaveContactSession(umodels.User{
		ID:        contact.ID,
		Email:     null.StringFrom(contact.Email),
		FirstName: contact.FirstName,
		LastName:  contact.LastName,
		Type:      umodels.UserTypeContact,
	}, r); err != nil {
		app.lo.Error("error creating portal session", "contact_id", contact.ID, "error", err)
		return r.RedirectURI("/portal/login?error=session", fasthttp.StatusFound, nil, "")
	}

	if err := app.auth.SetCSRFCookie(r); err != nil {
		app.lo.Error("error setting portal csrf cookie", "error", err)
	}
	return r.RedirectURI("/portal", fasthttp.StatusFound, nil, "")
}

// handlePortalLogout destroys the portal session.
func handlePortalLogout(r *fastglue.Request) error {
	app := r.Context.(*App)
	if err := app.auth.DestroyContactSession(r); err != nil {
		app.lo.Error("error destroying portal session", "error", err)
	}
	return r.SendEnvelope(map[string]any{"logged_out": true})
}

// handlePortalMe returns the signed-in contact.
func handlePortalMe(r *fastglue.Request) error {
	contact, err := getPortalContact(r)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(contact)
}

// handlePortalListConversations lists the contact's conversations.
func handlePortalListConversations(r *fastglue.Request) error {
	app := r.Context.(*App)
	contact, err := getPortalContact(r)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	args := r.RequestCtx.QueryArgs()
	status := string(args.Peek("status"))
	query := string(args.Peek("q"))
	page, _ := strconv.Atoi(string(args.Peek("page")))
	pageSize, _ := strconv.Atoi(string(args.Peek("per_page")))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = portalDefaultPage
	}
	if pageSize > portalMaxPageSize {
		pageSize = portalMaxPageSize
	}

	items, total, err := app.portal.ListConversations(contact.ID, status, query, page, pageSize)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(map[string]any{
		"results":  items,
		"total":    total,
		"page":     page,
		"per_page": pageSize,
	})
}

// handlePortalGetConversation returns one of the contact's conversations with public messages.
func handlePortalGetConversation(r *fastglue.Request) error {
	app := r.Context.(*App)
	contact, err := getPortalContact(r)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	uuid := r.RequestCtx.UserValue("uuid").(string)
	conv, err := app.portal.GetConversation(contact.ID, uuid)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(conv)
}

// handlePortalMarkSeen advances the contact's last-seen timestamp on a conversation.
func handlePortalMarkSeen(r *fastglue.Request) error {
	app := r.Context.(*App)
	contact, err := getPortalContact(r)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	uuid := r.RequestCtx.UserValue("uuid").(string)
	if err := app.portal.MarkSeen(contact.ID, uuid); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(map[string]any{"seen": true})
}

// portalBaseURL returns the externally reachable base URL of the app.
func portalBaseURL(r *fastglue.Request, app *App) string {
	if c, ok := app.consts.Load().(*constants); ok && c != nil && c.AppBaseURL != "" {
		return strings.TrimRight(c.AppBaseURL, "/")
	}
	scheme := "http"
	if r.RequestCtx.IsTLS() {
		scheme = "https"
	}
	return scheme + "://" + string(r.RequestCtx.Host())
}

// clientIP returns the client IP using the configured header.
func clientIP(r *fastglue.Request) string {
	return httputil.ClientIP(r.RequestCtx, r.Context.(*App).clientIPHeader)
}
