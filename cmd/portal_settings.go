package main

import (
	"net/url"
	"strings"
	"time"

	"github.com/abhinavxd/libredesk/internal/envelope"
	"github.com/abhinavxd/libredesk/internal/setting/models"
	"github.com/zerodha/fastglue"
)

// handleGetPortalSettings returns the portal settings.
func handleGetPortalSettings(r *fastglue.Request) error {
	app := r.Context.(*App)

	all, err := app.setting.GetAll()
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(all.Portal)
}

// handleUpdatePortalSettings updates the portal settings and reloads the manager.
func handleUpdatePortalSettings(r *fastglue.Request) error {
	app := r.Context.(*App)

	var req models.Portal
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(400, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}
	if err := validatePortalSettings(app, &req); err != nil {
		return sendErrorEnvelope(r, err)
	}

	if err := app.setting.Update(req); err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := reloadSettings(app); err != nil {
		return sendErrorEnvelope(r, err)
	}
	app.portal.Reload(portalConfigFromKo())

	return handleGetPortalSettings(r)
}

// validatePortalSettings normalizes and validates the submitted portal settings.
func validatePortalSettings(app *App, req *models.Portal) error {
	if req.DefaultInboxID > 0 {
		if _, err := app.portal.GetInbox(req.DefaultInboxID); err != nil {
			return err
		}
	}

	req.MagicLinkTTL = strings.TrimSpace(req.MagicLinkTTL)
	if req.MagicLinkTTL == "" {
		req.MagicLinkTTL = "15m"
	}
	if d, err := time.ParseDuration(req.MagicLinkTTL); err != nil || d < time.Minute {
		return envelope.NewError(envelope.InputError, app.i18n.T("validation.invalidDuration"), nil)
	}

	req.SessionLifetime = strings.TrimSpace(req.SessionLifetime)
	if req.SessionLifetime == "" {
		req.SessionLifetime = "720h"
	}
	if d, err := time.ParseDuration(req.SessionLifetime); err != nil || d < time.Hour {
		return envelope.NewError(envelope.InputError, app.i18n.T("validation.invalidDuration"), nil)
	}

	req.JWTSecret = strings.TrimSpace(req.JWTSecret)
	if req.JWTEnabled && len(req.JWTSecret) < 32 {
		return envelope.NewError(envelope.InputError, app.i18n.T("portal.jwtSecretWeak"), nil)
	}

	req.RootURL = strings.TrimRight(strings.TrimSpace(req.RootURL), "/")
	if req.RootURL != "" {
		u, err := url.Parse(req.RootURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return envelope.NewError(envelope.InputError, app.i18n.T("portal.invalidRootURL"), nil)
		}
	}
	return nil
}
