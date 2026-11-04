package webauth

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/server/webtls"
	"github.com/elentok/gx/server/wire"
)

// Refusal reasons of the web login routes (ADR 0028).
const (
	webLoginCookiePrefix     = "__Host-gx_login"
	webLoginCookieMaxAge     = int(webLoginIdleLimit / time.Second)
	webAuthBodyMax           = 4 << 10
	webAuthPathState         = "/v1/web/auth"
	webAuthPathSetupStart    = "/v1/web/auth/setup-start"
	webAuthPathSetupPassword = "/v1/web/auth/setup-password"
	webAuthPathSetupFinish   = "/v1/web/auth/setup-finish"
	webAuthPathLogin         = "/v1/web/auth/login"
	webAuthPathAddAuth       = "/v1/web/auth/add-authenticator"
	webAuthPathSignOut       = "/v1/web/auth/sign-out"
	webAuthPathLogins        = "/v1/web/logins"
	webAuthPathLoginRename   = "/v1/web/logins/rename"
	webAuthPathLoginRevoke   = "/v1/web/logins/revoke"
	webAuthPathPassword      = "/v1/web/account/password"
	webAuthPathResetBegin    = "/v1/web/account/authenticator/begin"
	webAuthPathResetFinish   = "/v1/web/account/authenticator/finish"
	webAuthStateSetup        = "setup"
	webAuthStateSignedOut    = "signed-out"
	webAuthStateSignedIn     = "signed-in"
	webLoginCookieRenewEvery = time.Minute
)

// Host is what the gate needs from the web server around it.
type Host interface {
	// IsWebRead reports whether the web mux serves r as a read.
	IsWebRead(r *http.Request) bool
	// CloseLogins ends the open event streams of the given web logins.
	CloseLogins(ids ...string)
}

// Config holds what NewGate needs.
type Config struct {
	Store    *WebAuth
	Cookie   string                   // the login cookie's name
	TOTPOn   func() bool              // whether login asks for a TOTP code (config server.web.totp)
	RootCert func() *x509.Certificate // the web root certificate
	Log      *slog.Logger
	Now      func() time.Time
	Host     Host
}

// Gate is the web login gate: the login store (embedded), the cookie checks and
// the login routes.
type Gate struct {
	*WebAuth
	cookie   string
	totpOn   func() bool
	rootCert func() *x509.Certificate
	log      *slog.Logger
	now      func() time.Time
	host     Host
	renewed  sync.Map // map web login id -> time its cookie was last renewed
}

// NewGate returns the gate over cfg.Store.
func NewGate(cfg Config) *Gate {
	return &Gate{WebAuth: cfg.Store, cookie: cfg.Cookie, totpOn: cfg.TOTPOn, rootCert: cfg.RootCert, log: cfg.Log, now: cfg.Now, host: cfg.Host}
}

// RenewedCount counts the web logins the cookie renewal table remembers. It is for tests.
func (g *Gate) RenewedCount() int {
	n := 0
	g.renewed.Range(func(_, _ any) bool { n++; return true })
	return n
}

// TOTPOn is whether login asks for a TOTP code (config server.web.totp).
func (g *Gate) TOTPOn() bool { return g.totpOn() }

type webAuthResult struct {
	Refused     bool   `json:"refused,omitempty"`
	Reason      string `json:"reason,omitempty"`
	WaitSeconds int    `json:"wait_seconds,omitempty"`
	// The fields below carry a secret: setup step 1 and the "authenticator needed" login result.
	AuthenticatorNeeded bool   `json:"authenticator_needed,omitempty"`
	PendingID           string `json:"pending_id,omitempty"`
	Secret              string `json:"secret,omitempty"`
	URI                 string `json:"uri,omitempty"`
}

type webAuthState struct {
	State   string `json:"state"`
	LoginID string `json:"login_id,omitempty"`
	// TOTPOn is whether login asks for a code (config server.web.totp); SetupCodeRequired
	// is whether it also needs the Setup code (after the CLI authenticator reset).
	TOTPOn            bool `json:"totp_on"`
	SetupCodeRequired bool `json:"setup_code_required"`
	// The root certificate's fingerprint and download paths: public, so the login and
	// setup pages can show them before anyone signs in.
	Fingerprint string `json:"fingerprint"`
	ProfilePath string `json:"profile_path"`
	CerPath     string `json:"cer_path"`
}

type webAuthBody struct {
	Code     string `json:"code"`
	Password string `json:"password"`
	TOTPCode string `json:"totp_code"`
	// PendingID names the secret an "authenticator needed" login handed out.
	PendingID string `json:"pending_id"`
	// SetupCode is the Setup code a login needs after the CLI reset.
	SetupCode string `json:"setup_code"`
	// ID and Name pick a web login and give it a new name (the logins routes).
	ID   string `json:"id"`
	Name string `json:"name"`
	// NewPassword is the password change password saves.
	NewPassword string `json:"new_password"`
}

type webLoginEntry struct {
	wire.WebLogin
	ThisDevice bool `json:"this_device"`
}

type webLoginList struct {
	Logins []webLoginEntry `json:"logins"`
}

type webLoginKey struct{}

// LoginIDFrom returns the id of the web login a gated request came in with.
func LoginIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(webLoginKey{}).(string)
	return id
}

// LoginCookieName names the web login cookie after the listener's port: browsers
// share cookies across ports on one host, so servers on 127.0.0.1:A and :B would
// otherwise overwrite each other's session.
func LoginCookieName(ln net.Listener) string {
	if _, port, err := net.SplitHostPort(ln.Addr().String()); err == nil && port != "" {
		return webLoginCookiePrefix + "_" + port
	}
	return webLoginCookiePrefix
}

// DropLogins ends the streams of removed web logins and forgets their cookie
// renewals.
func (g *Gate) DropLogins(ids ...string) {
	for _, id := range ids {
		g.renewed.Delete(id)
	}
	g.host.CloseLogins(ids...)
}

func (g *Gate) setLoginCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: g.cookie, Value: value, Path: "/", MaxAge: maxAge,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

// RemoteIP is the IP of the request's peer, without the port.
func RemoteIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// webLoginFor finds the web login behind the request's cookie.
func (g *Gate) webLoginFor(r *http.Request) (wire.WebLogin, string, bool) {
	c, err := r.Cookie(g.cookie)
	if err != nil {
		return wire.WebLogin{}, "", false
	}
	login, ok := g.Lookup(c.Value, RemoteIP(r))
	return login, c.Value, ok
}

// RequireLogin lets a request through with a valid web login cookie and
// renews the cookie at most once a minute. Any other request goes to fallback.
func (g *Gate) RequireLogin(next, fallback http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		login, value, ok := g.webLoginFor(r)
		if !ok {
			fallback.ServeHTTP(w, r)
			return
		}
		now := g.now()
		if last, seen := g.renewed.Load(login.ID); !seen || now.Sub(last.(time.Time)) >= webLoginCookieRenewEvery {
			g.renewed.Store(login.ID, now)
			g.setLoginCookie(w, value, webLoginCookieMaxAge)
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), webLoginKey{}, login.ID)))
	})
}

// Deny serves what needs no login (the page and the assets) and answers 401
// to everything else.
func (g *Gate) Deny(mux http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if g.host.IsWebRead(r) && !strings.HasPrefix(r.URL.Path, "/v1/") {
			mux.ServeHTTP(w, r)
			return
		}
		unauthorized(w)
	})
}

func (g *Gate) webAuthStateRead(w http.ResponseWriter, r *http.Request) {
	totp := g.totpOn()
	state := webAuthState{
		TOTPOn:            totp,
		SetupCodeRequired: totp && g.AuthenticatorResetPending(),
		Fingerprint:       webtls.RootFingerprint(g.rootCert().Raw),
		ProfilePath:       webtls.RootProfilePath,
		CerPath:           webtls.RootCerPath,
	}
	if login, _, ok := g.webLoginFor(r); ok {
		state.State, state.LoginID = webAuthStateSignedIn, login.ID
		wire.WriteJSON(w, state)
		return
	}
	pending, _, err := g.SetupState()
	if err != nil {
		http.Error(w, "web auth unavailable", http.StatusInternalServerError)
		return
	}
	state.State = webAuthStateSignedOut
	if pending {
		state.State = webAuthStateSetup
	}
	wire.WriteJSON(w, state)
}

func readWebAuthBody(w http.ResponseWriter, r *http.Request) (webAuthBody, bool) {
	var body webAuthBody
	r.Body = http.MaxBytesReader(w, r.Body, webAuthBodyMax)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad JSON", http.StatusBadRequest)
		return body, false
	}
	return body, true
}

// LogEvent writes one line per web auth event. It takes only the IP, the
// login name and a reason kind: never a request body, so no secret can reach it.
func (g *Gate) LogEvent(event, ip, name, reason string) {
	attrs := []any{"ip", ip}
	if name != "" {
		attrs = append(attrs, "name", name)
	}
	if reason != "" {
		g.log.Warn(event, append(attrs, "reason", reason)...)
		return
	}
	g.log.Info(event, attrs...)
}

func (g *Gate) webLoginName(id string) string {
	for _, l := range g.List() {
		if l.ID == id {
			return l.Name
		}
	}
	return ""
}

// logWebLogin logs a login event under the name of the web login the cookie opens.
func (g *Gate) logWebLogin(event, ip, cookie string) {
	login, _ := g.Lookup(cookie, ip)
	g.LogEvent(event, ip, login.Name, "")
}

// refuseWebAuth answers a store error with its refusal and logs it. Anything
// unknown is a 500.
func (g *Gate) refuseWebAuth(w http.ResponseWriter, r *http.Request, event string, err error, wrong string) {
	var wait *WebAuthWaitError
	reason := ""
	switch {
	case errors.As(err, &wait):
		g.LogEvent(event, RemoteIP(r), "", wire.ReasonRateLimited)
		wire.WriteJSON(w, webAuthResult{Refused: true, Reason: wire.ReasonRateLimited, WaitSeconds: wait.Seconds})
		return
	case errors.Is(err, ErrTOTPMismatch):
		reason = wire.ReasonTOTPMismatch
	case errors.Is(err, ErrWebAuthRefused):
		reason = wrong
	case errors.Is(err, ErrSetupNotPending):
		reason = wire.ReasonSetupNotPending
	case errors.Is(err, ErrPasswordTooShort):
		reason = wire.ReasonPasswordTooShort
	case errors.Is(err, ErrSetupCodeRequired):
		reason = wire.ReasonSetupCodeRequired
	default:
		g.log.Error("web auth failed", "event", event, "ip", RemoteIP(r), "err", err)
		http.Error(w, "web auth unavailable", http.StatusInternalServerError)
		return
	}
	g.LogEvent(event, RemoteIP(r), "", reason)
	wire.WriteJSON(w, webAuthResult{Refused: true, Reason: reason})
}

func (g *Gate) webSetupStart(w http.ResponseWriter, r *http.Request) {
	body, ok := readWebAuthBody(w, r)
	if !ok {
		return
	}
	secret, err := g.SetupStart(g.totpOn(), body.Code, RemoteIP(r))
	if err != nil {
		g.refuseWebAuth(w, r, "web setup refused", err, wire.ReasonSetupCodeWrong)
		return
	}
	res := webAuthResult{Secret: secret}
	if secret != "" {
		res.URI = totpURI(secret, totpAccountLabel)
	}
	wire.WriteJSON(w, res)
}

// webSetupPassword checks the password and keeps it for setup-finish, so the browser need not hold it.
func (g *Gate) webSetupPassword(w http.ResponseWriter, r *http.Request) {
	body, ok := readWebAuthBody(w, r)
	if !ok {
		return
	}
	if err := g.SetupPassword(body.Code, body.Password, RemoteIP(r)); err != nil {
		g.refuseWebAuth(w, r, "web setup refused", err, wire.ReasonSetupCodeWrong)
		return
	}
	wire.WriteJSON(w, webAuthResult{})
}

func (g *Gate) webSetupFinish(w http.ResponseWriter, r *http.Request) {
	body, ok := readWebAuthBody(w, r)
	if !ok {
		return
	}
	cookie, err := g.SetupFinish(g.totpOn(), body.Code, body.Password, body.TOTPCode, RemoteIP(r), r.UserAgent())
	if err != nil {
		g.refuseWebAuth(w, r, "web setup refused", err, wire.ReasonSetupCodeWrong)
		return
	}
	g.logWebLogin("web login", RemoteIP(r), cookie)
	g.setLoginCookie(w, cookie, webLoginCookieMaxAge)
	wire.WriteJSON(w, webAuthResult{})
}

func (g *Gate) webLoginWrite(w http.ResponseWriter, r *http.Request) {
	body, ok := readWebAuthBody(w, r)
	if !ok {
		return
	}
	cookie, err := g.Login(g.totpOn(), body.Password, body.TOTPCode, body.SetupCode, RemoteIP(r), r.UserAgent())
	var needed *AuthenticatorNeededError
	if errors.As(err, &needed) {
		wire.WriteJSON(w, webAuthResult{AuthenticatorNeeded: true, PendingID: needed.PendingID, Secret: needed.Secret, URI: needed.URI})
		return
	}
	if err != nil {
		g.refuseWebAuth(w, r, "web login refused", err, wire.ReasonWrongCredentials)
		return
	}
	g.logWebLogin("web login", RemoteIP(r), cookie)
	g.setLoginCookie(w, cookie, webLoginCookieMaxAge)
	wire.WriteJSON(w, webAuthResult{})
}

// webAddAuthenticator saves the secret a login handed out and signs the browser in.
func (g *Gate) webAddAuthenticator(w http.ResponseWriter, r *http.Request) {
	body, ok := readWebAuthBody(w, r)
	if !ok {
		return
	}
	if !g.totpOn() {
		wire.WriteJSON(w, webAuthResult{Refused: true, Reason: wire.ReasonTOTPOff})
		return
	}
	ip := RemoteIP(r)
	cookie, err := g.AddAuthenticator(body.PendingID, body.TOTPCode, ip, r.UserAgent())
	if err != nil {
		g.refuseWebAuth(w, r, "web login refused", err, wire.ReasonWrongCredentials)
		return
	}
	g.logWebLogin("web login", ip, cookie)
	g.setLoginCookie(w, cookie, webLoginCookieMaxAge)
	wire.WriteJSON(w, webAuthResult{})
}

// webLoginsList lists the web logins and marks the one the request came in with.
func (g *Gate) webLoginsList(w http.ResponseWriter, r *http.Request) {
	this := LoginIDFrom(r.Context())
	out := webLoginList{Logins: []webLoginEntry{}}
	for _, l := range g.List() {
		out.Logins = append(out.Logins, webLoginEntry{WebLogin: l, ThisDevice: l.ID == this})
	}
	wire.WriteJSON(w, out)
}

// refuseWebLogin answers a rename or revoke error with its refusal and logs it.
func (g *Gate) refuseWebLogin(w http.ResponseWriter, r *http.Request, event string, err error) {
	reason := ""
	switch {
	case errors.Is(err, ErrWebLoginNotFound):
		reason = wire.ReasonLoginNotFound
	case errors.Is(err, ErrWebLoginNameInvalid):
		reason = wire.ReasonLoginNameInvalid
	default:
		g.log.Error("web auth failed", "event", event, "ip", RemoteIP(r), "err", err)
		http.Error(w, "web auth unavailable", http.StatusInternalServerError)
		return
	}
	g.LogEvent(event, RemoteIP(r), "", reason)
	wire.WriteJSON(w, webAuthResult{Refused: true, Reason: reason})
}

func (g *Gate) webLoginRename(w http.ResponseWriter, r *http.Request) {
	body, ok := readWebAuthBody(w, r)
	if !ok {
		return
	}
	if err := g.RenameLogin(body.ID, body.Name); err != nil {
		g.refuseWebLogin(w, r, "web login rename refused", err)
		return
	}
	g.LogEvent("web login renamed", RemoteIP(r), body.Name, "")
	wire.WriteJSON(w, webAuthResult{})
}

// webLoginRevoke signs one web login out and ends its event streams.
func (g *Gate) webLoginRevoke(w http.ResponseWriter, r *http.Request) {
	body, ok := readWebAuthBody(w, r)
	if !ok {
		return
	}
	if err := g.Revoke(body.ID, RemoteIP(r)); err != nil {
		g.refuseWebLogin(w, r, "web login revoke refused", err)
		return
	}
	wire.WriteJSON(w, webAuthResult{})
}

// Revoke signs one web login out, ends its event streams and logs the
// revoke under ip. The web route and the CLI both go through it.
func (g *Gate) Revoke(id, ip string) error {
	name := g.webLoginName(id)
	removed, err := g.RevokeLogin(id)
	if err != nil {
		return err
	}
	g.LogEvent("web login revoked", ip, name, "")
	g.DropLogins(removed...)
	return nil
}

// webChangePassword saves a new password and signs out every other web login.
func (g *Gate) webChangePassword(w http.ResponseWriter, r *http.Request) {
	body, ok := readWebAuthBody(w, r)
	if !ok {
		return
	}
	this := LoginIDFrom(r.Context())
	name := g.webLoginName(this)
	removed, err := g.ChangePassword(g.totpOn(), body.Password, body.TOTPCode, body.NewPassword, this, RemoteIP(r))
	if err != nil {
		g.refuseWebAuth(w, r, "web password change refused", err, wire.ReasonWrongCredentials)
		return
	}
	g.LogEvent("web password changed", RemoteIP(r), name, "")
	g.DropLogins(removed...)
	wire.WriteJSON(w, webAuthResult{})
}

// webAuthenticatorBegin hands out a new secret once the password and a code from
// the current app check out.
func (g *Gate) webAuthenticatorBegin(w http.ResponseWriter, r *http.Request) {
	body, ok := readWebAuthBody(w, r)
	if !ok {
		return
	}
	if !g.totpOn() {
		wire.WriteJSON(w, webAuthResult{Refused: true, Reason: wire.ReasonTOTPOff})
		return
	}
	secret, err := g.AuthenticatorResetBegin(true, body.Password, body.TOTPCode, RemoteIP(r))
	if err != nil {
		g.refuseWebAuth(w, r, "web authenticator reset refused", err, wire.ReasonWrongCredentials)
		return
	}
	wire.WriteJSON(w, webAuthResult{Secret: secret, URI: totpURI(secret, totpAccountLabel)})
}

// webAuthenticatorFinish saves the new secret once a code from it checks out and
// signs out every other web login.
func (g *Gate) webAuthenticatorFinish(w http.ResponseWriter, r *http.Request) {
	body, ok := readWebAuthBody(w, r)
	if !ok {
		return
	}
	if !g.totpOn() {
		wire.WriteJSON(w, webAuthResult{Refused: true, Reason: wire.ReasonTOTPOff})
		return
	}
	this := LoginIDFrom(r.Context())
	name := g.webLoginName(this)
	removed, err := g.AuthenticatorResetFinish(body.TOTPCode, this, RemoteIP(r))
	if err != nil {
		g.refuseWebAuth(w, r, "web authenticator reset refused", err, wire.ReasonTOTPMismatch)
		return
	}
	g.LogEvent("web authenticator reset", RemoteIP(r), name, "")
	g.DropLogins(removed...)
	wire.WriteJSON(w, webAuthResult{})
}

// webSignOut drops this web login, ends its streams and clears the cookie.
func (g *Gate) webSignOut(w http.ResponseWriter, r *http.Request) {
	id := LoginIDFrom(r.Context())
	if id != "" {
		name := g.webLoginName(id)
		removed, err := g.RevokeLogin(id)
		if err != nil && !errors.Is(err, ErrWebLoginNotFound) {
			http.Error(w, "web auth unavailable", http.StatusInternalServerError)
			return
		}
		g.LogEvent("web sign out", RemoteIP(r), name, "")
		g.DropLogins(removed...)
	}
	g.setLoginCookie(w, "", -1)
	wire.WriteJSON(w, webAuthResult{})
}

// Routes are the web-only login routes. The gate resolves lazily: it is nil
// until the web listener is bound, and the routes are listed before that.
func Routes(gate func() *Gate) []wire.Route {
	h := func(f func(*Gate) http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { f(gate())(w, r) }
	}
	return []wire.Route{
		{Pattern: "GET " + webAuthPathState, Web: wire.WebPublic, WebHandler: h(func(g *Gate) http.HandlerFunc { return g.webAuthStateRead })},
		{Pattern: "POST " + webAuthPathSetupStart, Web: wire.WebPublic, WebHandler: h(func(g *Gate) http.HandlerFunc { return g.webSetupStart })},
		{Pattern: "POST " + webAuthPathSetupPassword, Web: wire.WebPublic, WebHandler: h(func(g *Gate) http.HandlerFunc { return g.webSetupPassword })},
		{Pattern: "POST " + webAuthPathSetupFinish, Web: wire.WebPublic, WebHandler: h(func(g *Gate) http.HandlerFunc { return g.webSetupFinish })},
		{Pattern: "POST " + webAuthPathLogin, Web: wire.WebPublic, WebHandler: h(func(g *Gate) http.HandlerFunc { return g.webLoginWrite })},
		{Pattern: "POST " + webAuthPathAddAuth, Web: wire.WebPublic, WebHandler: h(func(g *Gate) http.HandlerFunc { return g.webAddAuthenticator })},
		{Pattern: "POST " + webAuthPathSignOut, Web: wire.WebWrite, WebHandler: h(func(g *Gate) http.HandlerFunc { return g.webSignOut })},
		{Pattern: "POST " + webAuthPathLogins, Web: wire.WebWrite, WebHandler: h(func(g *Gate) http.HandlerFunc { return g.webLoginsList })},
		{Pattern: "POST " + webAuthPathLoginRename, Web: wire.WebWrite, WebHandler: h(func(g *Gate) http.HandlerFunc { return g.webLoginRename })},
		{Pattern: "POST " + webAuthPathLoginRevoke, Web: wire.WebWrite, WebHandler: h(func(g *Gate) http.HandlerFunc { return g.webLoginRevoke })},
		{Pattern: "POST " + webAuthPathPassword, Web: wire.WebWrite, WebHandler: h(func(g *Gate) http.HandlerFunc { return g.webChangePassword })},
		{Pattern: "POST " + webAuthPathResetBegin, Web: wire.WebWrite, WebHandler: h(func(g *Gate) http.HandlerFunc { return g.webAuthenticatorBegin })},
		{Pattern: "POST " + webAuthPathResetFinish, Web: wire.WebWrite, WebHandler: h(func(g *Gate) http.HandlerFunc { return g.webAuthenticatorFinish })},
	}
}
