package server

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"
)

// Refusal reasons of the web login routes (ADR 0028).
const (
	ReasonWrongCredentials   = "wrong-credentials"
	ReasonSetupCodeWrong     = "setup-code-wrong"
	ReasonSetupCodeRequired  = "setup-code-required"
	ReasonSetupNotPending    = "setup-not-pending"
	ReasonTOTPMismatch       = "totp-mismatch"
	ReasonPasswordTooShort   = "password-too-short"
	ReasonTOTPOff            = "totp-off"
	ReasonLoginNotFound      = "login-not-found"
	ReasonLoginNameInvalid   = "login-name-invalid"
	webLoginCookiePrefix     = "__Host-gx_login"
	webLoginCookieMaxAge     = int(webLoginIdleLimit / time.Second)
	webAuthBodyMax           = 4 << 10
	webAuthPathState         = "/v1/web/auth"
	webAuthPathSetupStart    = "/v1/web/auth/setup-start"
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

// webTOTPOn is whether login asks for a TOTP code (config server.web.totp).
func (s *Server) webTOTPOn() bool { return !s.settings.Load().WebTOTPOff }

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
	WebLogin
	ThisDevice bool `json:"this_device"`
}

type webLoginList struct {
	Logins []webLoginEntry `json:"logins"`
}

type webLoginKey struct{}

// webLoginIDFrom returns the id of the web login a gated request came in with.
func webLoginIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(webLoginKey{}).(string)
	return id
}

// loginCookieName names the web login cookie after the listener's port: browsers
// share cookies across ports on one host, so servers on 127.0.0.1:A and :B would
// otherwise overwrite each other's session.
func loginCookieName(ln net.Listener) string {
	if _, port, err := net.SplitHostPort(ln.Addr().String()); err == nil && port != "" {
		return webLoginCookiePrefix + "_" + port
	}
	return webLoginCookiePrefix
}

func (s *Server) webNow() time.Time {
	if s.cfg.Clock != nil {
		return s.cfg.Clock()
	}
	return time.Now()
}

func (s *Server) setLoginCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: s.webLoginCookie, Value: value, Path: "/", MaxAge: maxAge,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

func remoteIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// webLoginFor finds the web login behind the request's cookie.
func (s *Server) webLoginFor(r *http.Request) (WebLogin, string, bool) {
	c, err := r.Cookie(s.webLoginCookie)
	if err != nil {
		return WebLogin{}, "", false
	}
	login, ok := s.webAuth.Lookup(c.Value, remoteIP(r))
	return login, c.Value, ok
}

// requireWebLogin lets a request through with a valid web login cookie and
// renews the cookie at most once a minute. Any other request goes to fallback.
func (s *Server) requireWebLogin(next, fallback http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		login, value, ok := s.webLoginFor(r)
		if !ok {
			fallback.ServeHTTP(w, r)
			return
		}
		now := s.webNow()
		if last, seen := s.webRenewed.Load(login.ID); !seen || now.Sub(last.(time.Time)) >= webLoginCookieRenewEvery {
			s.webRenewed.Store(login.ID, now)
			s.setLoginCookie(w, value, webLoginCookieMaxAge)
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), webLoginKey{}, login.ID)))
	})
}

// denyWeb serves what needs no login (the page and the assets) and answers 401
// to everything else.
func (s *Server) denyWeb(mux http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && !strings.HasPrefix(r.URL.Path, "/v1/") {
			mux.ServeHTTP(w, r)
			return
		}
		unauthorized(w)
	})
}

func (s *Server) webAuthStateRead(w http.ResponseWriter, r *http.Request) {
	totp := s.webTOTPOn()
	state := webAuthState{TOTPOn: totp, SetupCodeRequired: totp && s.webAuth.AuthenticatorResetPending()}
	if login, _, ok := s.webLoginFor(r); ok {
		state.State, state.LoginID = webAuthStateSignedIn, login.ID
		writeJSON(w, state)
		return
	}
	pending, _, err := s.webAuth.SetupState()
	if err != nil {
		http.Error(w, "web auth unavailable", http.StatusInternalServerError)
		return
	}
	state.State = webAuthStateSignedOut
	if pending {
		state.State = webAuthStateSetup
	}
	writeJSON(w, state)
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

// logWebAuth writes one line per web auth event. It takes only the IP, the
// login name and a reason kind: never a request body, so no secret can reach it.
func (s *Server) logWebAuth(event, ip, name, reason string) {
	attrs := []any{"ip", ip}
	if name != "" {
		attrs = append(attrs, "name", name)
	}
	if reason != "" {
		s.log.Warn(event, append(attrs, "reason", reason)...)
		return
	}
	s.log.Info(event, attrs...)
}

func (s *Server) webLoginName(id string) string {
	for _, l := range s.webAuth.List() {
		if l.ID == id {
			return l.Name
		}
	}
	return ""
}

// logWebLogin logs a login event under the name of the web login the cookie opens.
func (s *Server) logWebLogin(event, ip, cookie string) {
	login, _ := s.webAuth.Lookup(cookie, ip)
	s.logWebAuth(event, ip, login.Name, "")
}

// refuseWebAuth answers a store error with its refusal and logs it. Anything
// unknown is a 500.
func (s *Server) refuseWebAuth(w http.ResponseWriter, r *http.Request, event string, err error, wrong string) {
	var wait *WebAuthWaitError
	reason := ""
	switch {
	case errors.As(err, &wait):
		s.logWebAuth(event, remoteIP(r), "", ReasonRateLimited)
		writeJSON(w, webAuthResult{Refused: true, Reason: ReasonRateLimited, WaitSeconds: wait.Seconds})
		return
	case errors.Is(err, ErrTOTPMismatch):
		reason = ReasonTOTPMismatch
	case errors.Is(err, ErrWebAuthRefused):
		reason = wrong
	case errors.Is(err, ErrSetupNotPending):
		reason = ReasonSetupNotPending
	case errors.Is(err, ErrPasswordTooShort):
		reason = ReasonPasswordTooShort
	case errors.Is(err, ErrSetupCodeRequired):
		reason = ReasonSetupCodeRequired
	default:
		s.log.Error("web auth failed", "event", event, "ip", remoteIP(r), "err", err)
		http.Error(w, "web auth unavailable", http.StatusInternalServerError)
		return
	}
	s.logWebAuth(event, remoteIP(r), "", reason)
	writeJSON(w, webAuthResult{Refused: true, Reason: reason})
}

func (s *Server) webSetupStart(w http.ResponseWriter, r *http.Request) {
	body, ok := readWebAuthBody(w, r)
	if !ok {
		return
	}
	secret, err := s.webAuth.SetupStart(s.webTOTPOn(), body.Code, remoteIP(r))
	if err != nil {
		s.refuseWebAuth(w, r, "web setup refused", err, ReasonSetupCodeWrong)
		return
	}
	res := webAuthResult{Secret: secret}
	if secret != "" {
		res.URI = totpURI(secret, totpAccountLabel)
	}
	writeJSON(w, res)
}

func (s *Server) webSetupFinish(w http.ResponseWriter, r *http.Request) {
	body, ok := readWebAuthBody(w, r)
	if !ok {
		return
	}
	cookie, err := s.webAuth.SetupFinish(s.webTOTPOn(), body.Code, body.Password, body.TOTPCode, remoteIP(r), r.UserAgent())
	if err != nil {
		s.refuseWebAuth(w, r, "web setup refused", err, ReasonSetupCodeWrong)
		return
	}
	s.logWebLogin("web login", remoteIP(r), cookie)
	s.setLoginCookie(w, cookie, webLoginCookieMaxAge)
	writeJSON(w, webAuthResult{})
}

func (s *Server) webLoginWrite(w http.ResponseWriter, r *http.Request) {
	body, ok := readWebAuthBody(w, r)
	if !ok {
		return
	}
	cookie, err := s.webAuth.Login(s.webTOTPOn(), body.Password, body.TOTPCode, body.SetupCode, remoteIP(r), r.UserAgent())
	var needed *AuthenticatorNeededError
	if errors.As(err, &needed) {
		writeJSON(w, webAuthResult{AuthenticatorNeeded: true, PendingID: needed.PendingID, Secret: needed.Secret, URI: needed.URI})
		return
	}
	if err != nil {
		s.refuseWebAuth(w, r, "web login refused", err, ReasonWrongCredentials)
		return
	}
	s.logWebLogin("web login", remoteIP(r), cookie)
	s.setLoginCookie(w, cookie, webLoginCookieMaxAge)
	writeJSON(w, webAuthResult{})
}

// webAddAuthenticator saves the secret a login handed out and signs the browser in.
func (s *Server) webAddAuthenticator(w http.ResponseWriter, r *http.Request) {
	body, ok := readWebAuthBody(w, r)
	if !ok {
		return
	}
	if !s.webTOTPOn() {
		writeJSON(w, webAuthResult{Refused: true, Reason: ReasonTOTPOff})
		return
	}
	ip := remoteIP(r)
	cookie, err := s.webAuth.AddAuthenticator(body.PendingID, body.TOTPCode, ip, r.UserAgent())
	if err != nil {
		s.refuseWebAuth(w, r, "web login refused", err, ReasonWrongCredentials)
		return
	}
	s.logWebLogin("web login", ip, cookie)
	s.setLoginCookie(w, cookie, webLoginCookieMaxAge)
	writeJSON(w, webAuthResult{})
}

// webLoginsList lists the web logins and marks the one the request came in with.
func (s *Server) webLoginsList(w http.ResponseWriter, r *http.Request) {
	this := webLoginIDFrom(r.Context())
	out := webLoginList{Logins: []webLoginEntry{}}
	for _, l := range s.webAuth.List() {
		out.Logins = append(out.Logins, webLoginEntry{WebLogin: l, ThisDevice: l.ID == this})
	}
	writeJSON(w, out)
}

// refuseWebLogin answers a rename or revoke error with its refusal and logs it.
func (s *Server) refuseWebLogin(w http.ResponseWriter, r *http.Request, event string, err error) {
	reason := ""
	switch {
	case errors.Is(err, ErrWebLoginNotFound):
		reason = ReasonLoginNotFound
	case errors.Is(err, ErrWebLoginNameInvalid):
		reason = ReasonLoginNameInvalid
	default:
		s.log.Error("web auth failed", "event", event, "ip", remoteIP(r), "err", err)
		http.Error(w, "web auth unavailable", http.StatusInternalServerError)
		return
	}
	s.logWebAuth(event, remoteIP(r), "", reason)
	writeJSON(w, webAuthResult{Refused: true, Reason: reason})
}

func (s *Server) webLoginRename(w http.ResponseWriter, r *http.Request) {
	body, ok := readWebAuthBody(w, r)
	if !ok {
		return
	}
	if err := s.webAuth.RenameLogin(body.ID, body.Name); err != nil {
		s.refuseWebLogin(w, r, "web login rename refused", err)
		return
	}
	s.logWebAuth("web login renamed", remoteIP(r), body.Name, "")
	writeJSON(w, webAuthResult{})
}

// webLoginRevoke signs one web login out and ends its event streams.
func (s *Server) webLoginRevoke(w http.ResponseWriter, r *http.Request) {
	body, ok := readWebAuthBody(w, r)
	if !ok {
		return
	}
	if err := s.revokeWebLogin(body.ID, remoteIP(r)); err != nil {
		s.refuseWebLogin(w, r, "web login revoke refused", err)
		return
	}
	writeJSON(w, webAuthResult{})
}

// revokeWebLogin signs one web login out, ends its event streams and logs the
// revoke under ip. The web route and the CLI both go through it.
func (s *Server) revokeWebLogin(id, ip string) error {
	name := s.webLoginName(id)
	removed, err := s.webAuth.RevokeLogin(id)
	if err != nil {
		return err
	}
	s.logWebAuth("web login revoked", ip, name, "")
	s.webStreams.closeLogins(removed...)
	return nil
}

// webChangePassword saves a new password and signs out every other web login.
func (s *Server) webChangePassword(w http.ResponseWriter, r *http.Request) {
	body, ok := readWebAuthBody(w, r)
	if !ok {
		return
	}
	this := webLoginIDFrom(r.Context())
	name := s.webLoginName(this)
	removed, err := s.webAuth.ChangePassword(s.webTOTPOn(), body.Password, body.TOTPCode, body.NewPassword, this, remoteIP(r))
	if err != nil {
		s.refuseWebAuth(w, r, "web password change refused", err, ReasonWrongCredentials)
		return
	}
	s.logWebAuth("web password changed", remoteIP(r), name, "")
	s.webStreams.closeLogins(removed...)
	writeJSON(w, webAuthResult{})
}

// webAuthenticatorBegin hands out a new secret once the password and a code from
// the current app check out.
func (s *Server) webAuthenticatorBegin(w http.ResponseWriter, r *http.Request) {
	body, ok := readWebAuthBody(w, r)
	if !ok {
		return
	}
	if !s.webTOTPOn() {
		writeJSON(w, webAuthResult{Refused: true, Reason: ReasonTOTPOff})
		return
	}
	secret, err := s.webAuth.AuthenticatorResetBegin(true, body.Password, body.TOTPCode, remoteIP(r))
	if err != nil {
		s.refuseWebAuth(w, r, "web authenticator reset refused", err, ReasonWrongCredentials)
		return
	}
	writeJSON(w, webAuthResult{Secret: secret, URI: totpURI(secret, totpAccountLabel)})
}

// webAuthenticatorFinish saves the new secret once a code from it checks out and
// signs out every other web login.
func (s *Server) webAuthenticatorFinish(w http.ResponseWriter, r *http.Request) {
	body, ok := readWebAuthBody(w, r)
	if !ok {
		return
	}
	if !s.webTOTPOn() {
		writeJSON(w, webAuthResult{Refused: true, Reason: ReasonTOTPOff})
		return
	}
	this := webLoginIDFrom(r.Context())
	name := s.webLoginName(this)
	removed, err := s.webAuth.AuthenticatorResetFinish(body.TOTPCode, this, remoteIP(r))
	if err != nil {
		s.refuseWebAuth(w, r, "web authenticator reset refused", err, ReasonTOTPMismatch)
		return
	}
	s.logWebAuth("web authenticator reset", remoteIP(r), name, "")
	s.webStreams.closeLogins(removed...)
	writeJSON(w, webAuthResult{})
}

// webSignOut drops this web login, ends its streams and clears the cookie.
func (s *Server) webSignOut(w http.ResponseWriter, r *http.Request) {
	id := webLoginIDFrom(r.Context())
	if id != "" {
		name := s.webLoginName(id)
		removed, err := s.webAuth.RevokeLogin(id)
		if err != nil && !errors.Is(err, ErrWebLoginNotFound) {
			http.Error(w, "web auth unavailable", http.StatusInternalServerError)
			return
		}
		s.logWebAuth("web sign out", remoteIP(r), name, "")
		s.webStreams.closeLogins(removed...)
	}
	s.setLoginCookie(w, "", -1)
	writeJSON(w, webAuthResult{})
}
