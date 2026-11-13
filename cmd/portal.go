package main

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/abhinavxd/libredesk/internal/attachment"
	cmodels "github.com/abhinavxd/libredesk/internal/conversation/models"
	"github.com/abhinavxd/libredesk/internal/envelope"
	"github.com/abhinavxd/libredesk/internal/httputil"
	notifier "github.com/abhinavxd/libredesk/internal/notification"
	"github.com/abhinavxd/libredesk/internal/portal"
	pmodels "github.com/abhinavxd/libredesk/internal/portal/models"
	"github.com/abhinavxd/libredesk/internal/stringutil"
	tmpl "github.com/abhinavxd/libredesk/internal/template"
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

	content, err := app.tmpl.RenderInMemoryTemplate(tmpl.TmplPortalMagicLink, map[string]any{
		"Link":   link,
		"Title":  app.i18n.T("portal.email.heading"),
		"Intro":  app.i18n.T("portal.email.intro"),
		"Button": app.i18n.T("portal.email.button"),
		"Hint":   app.i18n.T("portal.email.hint"),
	})
	if err != nil {
		app.lo.Warn("error rendering portal magic link template, using plain body", "error", err)
		content = app.i18n.Ts("portal.email.body", "link", link)
	}

	if inboxID := app.portal.Config().DefaultInboxID; inboxID > 0 {
		if err := app.conversation.SendTransientEmail(inboxID, 0, "", []string{contact.Email}, subject, content); err == nil {
			return
		} else {
			app.lo.Warn("error sending portal magic link via inbox, falling back to notification email", "inbox_id", inboxID, "error", err)
		}
	}

	if err := app.notifier.Send(notifier.Message{
		Provider:        notifier.ProviderEmail,
		RecipientEmails: []string{contact.Email},
		Subject:         subject,
		Content:         content,
		ContentType:     "html",
	}); err != nil {
		app.lo.Error("error queueing portal magic link email", "contact_id", contact.ID, "error", err)
	}
}

// handlePortalVerify confirms a magic-link login and starts a portal session.
// GET only validates the token and renders a confirmation page so email link
// scanners cannot burn single-use tokens; POST consumes the token.
func handlePortalVerify(r *fastglue.Request) error {
	app := r.Context.(*App)
	if !app.portal.Enabled() {
		return r.RedirectURI("/portal/login?error=disabled", fasthttp.StatusFound, nil, "")
	}

	token := strings.TrimSpace(string(r.RequestCtx.QueryArgs().Peek("token")))
	if r.RequestCtx.IsPost() {
		token = strings.TrimSpace(string(r.RequestCtx.PostArgs().Peek("token")))
	}
	if token == "" {
		return r.RedirectURI("/portal/login?error=invalid", fasthttp.StatusFound, nil, "")
	}

	if !r.RequestCtx.IsPost() {
		if _, err := app.portal.ValidateLoginToken(token); err != nil {
			return r.RedirectURI("/portal/login?error=invalid", fasthttp.StatusFound, nil, "")
		}
		return sendPortalVerifyPage(r, app, token)
	}

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

// sendPortalVerifyPage renders the one-click confirmation form for a magic link.
func sendPortalVerifyPage(r *fastglue.Request, app *App, token string) error {
	page := fmt.Sprintf(`<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<style>
body{background:#f4f4f5;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;color:#09090b}
.card{background:#fff;border:1px solid #e4e4e7;border-radius:8px;padding:32px;max-width:420px;width:calc(100%% - 48px)}
h1{font-size:20px;margin:0 0 12px}
p{font-size:14px;line-height:1.6;color:#52525b;margin:0 0 24px}
button{background:#18181b;color:#fafafa;border:0;border-radius:6px;padding:10px 20px;font-size:14px;font-weight:500;cursor:pointer}
</style>
</head>
<body>
<div class="card">
<h1>%s</h1>
<p>%s</p>
<form method="post" action="/api/v1/portal/auth/verify">
<input type="hidden" name="token" value="%s">
<button type="submit">%s</button>
</form>
</div>
</body>
</html>`,
		html.EscapeString(app.i18n.T("portal.verify.title")),
		html.EscapeString(app.i18n.T("portal.verify.title")),
		html.EscapeString(app.i18n.T("portal.verify.intro")),
		html.EscapeString(token),
		html.EscapeString(app.i18n.T("portal.verify.button")),
	)

	r.RequestCtx.Response.Header.Set("Content-Type", "text/html; charset=utf-8")
	r.RequestCtx.Response.Header.Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	r.RequestCtx.Response.Header.Set("Pragma", "no-cache")
	r.RequestCtx.Response.Header.Set("Referrer-Policy", "no-referrer")
	r.RequestCtx.Response.Header.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'")
	r.RequestCtx.Response.SetBodyString(page)
	return nil
}

// handlePortalAuthExchange exchanges a signed JWT for a portal session.
func handlePortalAuthExchange(r *fastglue.Request) error {
	app := r.Context.(*App)
	if !app.portal.Enabled() {
		return r.SendErrorEnvelope(http.StatusNotFound, app.i18n.T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}

	cfg := app.portal.Config()
	if !cfg.JWTEnabled || cfg.JWTSecret == "" {
		return r.SendErrorEnvelope(http.StatusNotFound, app.i18n.T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}

	var req struct {
		JWT string `json:"jwt"`
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(http.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}

	claims, err := portal.VerifyJWT(req.JWT, cfg.JWTSecret)
	if err != nil {
		app.lo.Warn("portal jwt rejected", "error", err)
		return r.SendErrorEnvelope(http.StatusUnauthorized, app.i18n.T("globals.terms.unAuthorized"), nil, envelope.UnauthorizedError)
	}

	var contact pmodels.Contact
	if claims.ExternalUserID != "" {
		contact, err = app.portal.GetContactByExternalID(claims.ExternalUserID)
	}
	if err != nil || contact.ID == 0 {
		contact, err = app.portal.GetContactByEmail(claims.Email)
	}
	if err != nil || contact.ID == 0 {
		app.lo.Warn("portal jwt contact not found", "external_user_id", claims.ExternalUserID, "email", claims.Email)
		return r.SendErrorEnvelope(http.StatusUnauthorized, app.i18n.T("globals.terms.unAuthorized"), nil, envelope.UnauthorizedError)
	}

	if err := app.auth.SaveContactSession(umodels.User{
		ID:        contact.ID,
		Email:     null.StringFrom(contact.Email),
		FirstName: contact.FirstName,
		LastName:  contact.LastName,
		Type:      umodels.UserTypeContact,
	}, r); err != nil {
		app.lo.Error("error creating portal session from jwt", "contact_id", contact.ID, "error", err)
		return r.SendErrorEnvelope(http.StatusInternalServerError, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	if err := app.auth.SetCSRFCookie(r); err != nil {
		app.lo.Error("error setting portal csrf cookie", "error", err)
	}
	return r.SendEnvelope(contact)
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

// handlePortalCreateConversation starts a new ticket from the portal.
func handlePortalCreateConversation(r *fastglue.Request) error {
	app := r.Context.(*App)
	contact, err := getPortalContact(r)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	var req struct {
		Subject string `json:"subject"`
		Message string `json:"message"`
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(http.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}

	subject := strings.TrimSpace(req.Subject)
	message := strings.TrimSpace(req.Message)
	if subject == "" {
		return r.SendErrorEnvelope(http.StatusBadRequest, app.i18n.Ts("globals.messages.required", "name", "subject"), nil, envelope.InputError)
	}
	if len(subject) > maxChatSubjectLength {
		return r.SendErrorEnvelope(http.StatusBadRequest, app.i18n.Ts("globals.messages.maxLength", "max", strconv.Itoa(maxChatSubjectLength)), nil, envelope.InputError)
	}
	if message == "" {
		return r.SendErrorEnvelope(http.StatusBadRequest, app.i18n.Ts("globals.messages.required", "name", "{globals.terms.message}"), nil, envelope.InputError)
	}
	if len(message) > maxChatMessageLength {
		return r.SendErrorEnvelope(http.StatusBadRequest, app.i18n.Ts("globals.messages.maxLength", "max", strconv.Itoa(maxChatMessageLength)), nil, envelope.InputError)
	}

	uuid, err := app.portal.CreateConversation(contact, subject, message)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(map[string]any{"uuid": uuid})
}

// handlePortalReply adds a public reply to one of the contact's conversations.
func handlePortalReply(r *fastglue.Request) error {
	app := r.Context.(*App)
	contact, err := getPortalContact(r)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(http.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return r.SendErrorEnvelope(http.StatusBadRequest, app.i18n.Ts("globals.messages.required", "name", "{globals.terms.message}"), nil, envelope.InputError)
	}
	if len(message) > maxChatMessageLength {
		return r.SendErrorEnvelope(http.StatusBadRequest, app.i18n.Ts("globals.messages.maxLength", "max", strconv.Itoa(maxChatMessageLength)), nil, envelope.InputError)
	}

	uuid := r.RequestCtx.UserValue("uuid").(string)
	messageUUID, err := app.portal.Reply(contact.ID, uuid, message)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(map[string]any{"uuid": messageUUID})
}

// handlePortalTranscript downloads a plain-text transcript of one of the contact's tickets.
func handlePortalTranscript(r *fastglue.Request) error {
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

	body := renderPortalTranscript(conv)
	filename := "ticket-" + stringutil.SanitizeFilename(conv.ReferenceNumber) + ".txt"
	r.RequestCtx.Response.Header.Set("Content-Type", "text/plain; charset=utf-8")
	r.RequestCtx.Response.Header.Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	r.RequestCtx.Response.Header.Set("Cache-Control", "no-store")
	r.RequestCtx.Response.SetBodyString(body)
	return nil
}

var transcriptTagPattern = regexp.MustCompile(`<[^>]*>`)

// renderPortalTranscript formats a conversation and its public messages as plain text.
func renderPortalTranscript(conv pmodels.Conversation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Subject: %s\n", conv.Subject)
	fmt.Fprintf(&b, "Reference: %s\n", conv.ReferenceNumber)
	fmt.Fprintf(&b, "Status: %s\n", conv.Status.Name)
	fmt.Fprintf(&b, "Inbox: %s\n", conv.InboxName)
	fmt.Fprintf(&b, "Created: %s\n", conv.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "Total time spent: %s\n", (time.Duration(conv.TimeSpentSeconds) * time.Second).String())
	b.WriteString("\n")

	for _, msg := range conv.Messages {
		author := msg.AuthorName
		if msg.SenderType == "contact" {
			author = "Customer"
		}
		fmt.Fprintf(&b, "[%s] %s\n", msg.CreatedAt.Format("2006-01-02 15:04"), author)
		b.WriteString(plainTranscriptText(msg.Content, msg.ContentType))
		b.WriteString("\n")
		for _, a := range msg.Attachments {
			fmt.Fprintf(&b, "Attachment: %s (%d bytes) %s\n", a.Name, a.Size, a.URL)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func plainTranscriptText(content, contentType string) string {
	if contentType != "html" {
		return content
	}
	text := transcriptTagPattern.ReplaceAllString(content, " ")
	return strings.TrimSpace(html.UnescapeString(text))
}

// handlePortalUploadAttachment adds a file as a message to one of the contact's conversations.
func handlePortalUploadAttachment(r *fastglue.Request) error {
	app := r.Context.(*App)
	contact, err := getPortalContact(r)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	uuid := r.RequestCtx.UserValue("uuid").(string)
	conv, err := app.conversation.GetConversation(0, uuid, "")
	if err != nil || conv.ContactID != contact.ID {
		return r.SendErrorEnvelope(http.StatusNotFound, app.i18n.T("validation.notFoundConversation"), nil, envelope.NotFoundError)
	}
	if err := app.portal.CanReply(conv); err != nil {
		return sendErrorEnvelope(r, err)
	}

	form, err := r.RequestCtx.MultipartForm()
	if err != nil {
		return r.SendErrorEnvelope(http.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}
	files, ok := form.File["files"]
	if !ok || len(files) == 0 {
		return r.SendErrorEnvelope(http.StatusBadRequest, app.i18n.T("validation.notFoundFile"), nil, envelope.InputError)
	}

	fileHeader := files[0]
	file, err := fileHeader.Open()
	if err != nil {
		return r.SendErrorEnvelope(http.StatusInternalServerError, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	defer file.Close()

	consts := app.consts.Load().(*constants)
	if bytesToMegabytes(fileHeader.Size) > float64(consts.MaxFileUploadSizeMB) {
		return r.SendErrorEnvelope(
			http.StatusRequestEntityTooLarge,
			app.i18n.Ts("media.fileSizeTooLarge", "size", fmt.Sprintf("%dMB", consts.MaxFileUploadSizeMB)),
			nil,
			envelope.GeneralError,
		)
	}

	srcFileName := stringutil.SanitizeFilename(fileHeader.Filename)
	srcExt := strings.TrimPrefix(strings.ToLower(filepath.Ext(srcFileName)), ".")
	if !slices.Contains(consts.AllowedUploadFileExtensions, "*") && !slices.Contains(consts.AllowedUploadFileExtensions, srcExt) {
		return r.SendErrorEnvelope(http.StatusBadRequest, app.i18n.T("media.fileTypeNotAllowed"), nil, envelope.InputError)
	}

	content, err := io.ReadAll(file)
	if err != nil {
		return r.SendErrorEnvelope(http.StatusInternalServerError, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	msg, err := app.portal.ProcessContactMessage(conv, cmodels.Message{
		ConversationUUID: conv.UUID,
		ConversationID:   conv.ID,
		SenderID:         contact.ID,
		Type:             cmodels.MessageIncoming,
		SenderType:       cmodels.SenderTypeContact,
		Status:           cmodels.MessageStatusReceived,
		Content:          "",
		ContentType:      cmodels.ContentTypeText,
		Private:          false,
		Attachments: attachment.Attachments{{
			Name:        srcFileName,
			ContentType: fileHeader.Header.Get("Content-Type"),
			Size:        int(fileHeader.Size),
			Content:     content,
			Disposition: attachment.DispositionAttachment,
		}},
	})
	if err != nil {
		app.lo.Error("error uploading portal attachment", "contact_id", contact.ID, "uuid", uuid, "error", err)
		return r.SendErrorEnvelope(http.StatusInternalServerError, app.i18n.T("globals.messages.errorUploadingFile"), nil, envelope.GeneralError)
	}
	return r.SendEnvelope(map[string]any{"uuid": msg.UUID})
}

// portalBaseURL returns the externally reachable base URL of the app.
func portalBaseURL(r *fastglue.Request, app *App) string {
	if root := app.portal.Config().RootURL; root != "" {
		return strings.TrimRight(root, "/")
	}
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
