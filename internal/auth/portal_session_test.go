package auth

import (
	"errors"
	"testing"
	"time"

	umodels "github.com/abhinavxd/libredesk/internal/user/models"
	"github.com/valyala/fasthttp"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/fastglue"
	"github.com/zerodha/simplesessions/v3"
)

func portalSessionRequest(cookie string) *fastglue.Request {
	r := &fastglue.Request{RequestCtx: &fasthttp.RequestCtx{}}
	var req fasthttp.Request
	r.RequestCtx.Init(&req, nil /** remoteAddr **/, nil /** logger **/)
	r.RequestCtx.Request.SetRequestURI("https://app.example.com/portal")
	if cookie != "" {
		r.RequestCtx.Request.Header.SetCookie("libredesk_portal", cookie)
	}
	return r
}

func savePortalSession(t *testing.T, a *Auth, contact umodels.User) string {
	t.Helper()
	r := portalSessionRequest("")
	if err := a.SaveContactSession(contact, r); err != nil {
		t.Fatal(err)
	}
	var cookie fasthttp.Cookie
	cookie.SetKey("libredesk_portal")
	if !r.RequestCtx.Response.Header.Cookie(&cookie) {
		t.Fatal("missing portal session cookie")
	}
	return string(cookie.Value())
}

func TestPortalContactSessionLifecycle(t *testing.T) {
	a, rd, _ := sessionAuthForTest(t, time.Hour)
	contact := umodels.User{ID: 42, Email: null.StringFrom("customer@example.com"), FirstName: "Ivan"}

	cookie := savePortalSession(t, a, contact)

	got, err := a.ValidateContactSession(portalSessionRequest(cookie))
	if err != nil || got.ID != 42 || got.Email.String != "customer@example.com" || got.FirstName != "Ivan" {
		t.Fatalf("portal session user=%+v err=%v", got, err)
	}

	if err := a.DestroyContactSession(portalSessionRequest(cookie)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ValidateContactSession(portalSessionRequest(cookie)); !errors.Is(err, simplesessions.ErrInvalidSession) {
		t.Fatalf("destroyed portal session accepted: err=%v", err)
	}

	// Session revocation by contact must remove every session and its index.
	first := savePortalSession(t, a, contact)
	second := savePortalSession(t, a, contact)
	if err := a.DestroyContactSessions(t.Context(), 42); err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{first, second} {
		if _, err := a.ValidateContactSession(portalSessionRequest(c)); !errors.Is(err, simplesessions.ErrInvalidSession) {
			t.Fatalf("revoked portal session accepted: err=%v", err)
		}
	}
	if n, err := rd.Exists(t.Context(), portalUserSessionKeyPrefix+"42").Result(); err != nil || n != 0 {
		t.Fatalf("portal session index remains: count=%d err=%v", n, err)
	}
}

func TestPortalSessionIsSeparateFromAgentSession(t *testing.T) {
	a, _, _ := sessionAuthForTest(t, time.Hour)
	agentCookie := saveTestSession(t, a, 1)
	cookie := savePortalSession(t, a, umodels.User{ID: 7, Email: null.StringFrom("customer@example.com")})

	if _, err := a.ValidateContactSession(portalSessionRequest(agentCookie)); !errors.Is(err, simplesessions.ErrInvalidSession) {
		t.Fatalf("agent cookie accepted by the portal: err=%v", err)
	}
	if _, err := a.ValidateSession(portalSessionRequest(cookie)); !errors.Is(err, simplesessions.ErrInvalidSession) {
		t.Fatalf("portal cookie accepted by the agent session: err=%v", err)
	}
}
