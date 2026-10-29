package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// ReasonWebRevokeFailed refuses a CLI revoke when the web logins could not be saved.
const ReasonWebRevokeFailed = "web-revoke-failed"

// ReasonWebPasswordFailed refuses a CLI password change when it could not be saved.
const ReasonWebPasswordFailed = "web-password-failed"

// ReasonWebOff refuses a web read when the web listener is off or failed to bind.
const ReasonWebOff = "web-off"

// ReasonWebNewRootFailed refuses a new-root request when the root could not be made or saved.
const ReasonWebNewRootFailed = "web-new-root-failed"

// ReasonWebResetFailed refuses a web reset when the account could not be cleared or saved.
const ReasonWebResetFailed = "web-reset-failed"

// ReasonWebAuthenticatorResetFailed refuses a CLI authenticator reset when it could not be saved.
const ReasonWebAuthenticatorResetFailed = "web-authenticator-reset-failed"

// WebInfo is the GET /v1/web/info payload behind `gx server web-url`.
type WebInfo struct {
	BaseURL string `json:"base_url,omitempty"`
	// Fingerprint is the SHA-256 of the root certificate, in 8 groups of 8.
	Fingerprint string `json:"fingerprint,omitempty"`
	// LeftOutHosts and LeftOutIPs are the names the leaf omits because the root does not permit them.
	LeftOutHosts []string `json:"left_out_hosts,omitempty"`
	LeftOutIPs   []string `json:"left_out_ips,omitempty"`
	// SetupPending is true until the first password is set; SetupCode is the Setup code while it is.
	SetupPending bool   `json:"setup_pending,omitempty"`
	SetupCode    string `json:"setup_code,omitempty"`
	// SetupExpires is when SetupCode stops working.
	SetupExpires time.Time `json:"setup_expires,omitzero"`
	Refused      bool      `json:"refused,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	Message      string    `json:"message,omitempty"`
}

func (s *Server) webInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.webInfoNow())
}

// webNewRoot makes a new root from the current allowed hosts and swaps root and
// leaf live. A bad Host header or token is turned away before this runs.
func (s *Server) webNewRoot(w http.ResponseWriter, _ *http.Request) {
	if s.web == nil {
		writeJSON(w, webOffInfo())
		return
	}
	if err := s.webCerts.newRoot(); err != nil {
		s.log.Error("new root failed", "err", err)
		writeJSON(w, WebInfo{Refused: true, Reason: ReasonWebNewRootFailed, Message: "could not make a new root: " + err.Error()})
		return
	}
	info := s.webInfoNow()
	s.log.Info("new root", "fingerprint", info.Fingerprint)
	writeJSON(w, info)
}

// webReset clears the web account, ends every web login's event streams and
// answers with the new Setup code. The socket is the only way in.
func (s *Server) webReset(w http.ResponseWriter, _ *http.Request) {
	if s.web == nil {
		writeJSON(w, webOffInfo())
		return
	}
	removed, err := s.webAuth.WebReset()
	if err != nil {
		s.log.Error("web reset failed", "err", err)
		writeJSON(w, WebInfo{Refused: true, Reason: ReasonWebResetFailed, Message: "could not reset the web account: " + err.Error()})
		return
	}
	s.webStreams.closeLogins(removed...)
	s.logWebAuth("web reset", "cli", "", "")
	writeJSON(w, s.webInfoNow())
}

// webResetAuthenticator is the CLI authenticator reset: it signs out every web
// login, ends their streams and answers with the new Setup code. The answer
// holds no TOTP secret.
func (s *Server) webResetAuthenticator(w http.ResponseWriter, _ *http.Request) {
	if s.web == nil {
		writeJSON(w, webOffInfo())
		return
	}
	removed, _, err := s.webAuth.ClearAuthenticator()
	if err != nil {
		s.log.Error("web authenticator reset failed", "err", err)
		writeJSON(w, WebInfo{Refused: true, Reason: ReasonWebAuthenticatorResetFailed, Message: "could not reset the authenticator: " + err.Error()})
		return
	}
	s.webStreams.closeLogins(removed...)
	s.logWebAuth("web authenticator reset", "cli", "", "")
	info := s.webInfoNow()
	s.addResetCode(&info)
	writeJSON(w, info)
}

// addResetCode puts the pending authenticator reset's Setup code on info.
func (s *Server) addResetCode(info *WebInfo) {
	code, expires, ok, err := s.webAuth.AuthenticatorResetCode()
	if err != nil {
		s.log.Error("web setup code unavailable", "err", err)
		return
	}
	if ok {
		info.SetupCode, info.SetupExpires = code, expires
	}
}

// WebLoginsResult is the payload of the web login list and revoke routes behind
// `gx server web-logins` and `web-revoke`. A revoke answers with no Logins.
type WebLoginsResult struct {
	Logins  []WebLogin `json:"logins,omitempty"`
	Refused bool       `json:"refused,omitempty"`
	Reason  string     `json:"reason,omitempty"`
	Message string     `json:"message,omitempty"`
}

func webOffLogins() WebLoginsResult {
	off := webOffInfo()
	return WebLoginsResult{Refused: true, Reason: off.Reason, Message: off.Message}
}

// webLoginListCLI answers the CLI with every web login.
func (s *Server) webLoginListCLI(w http.ResponseWriter, _ *http.Request) {
	if s.web == nil {
		writeJSON(w, webOffLogins())
		return
	}
	writeJSON(w, WebLoginsResult{Logins: s.webAuth.List()})
}

// webLoginRevokeCLI revokes one web login by id. An unknown id is a refusal.
func (s *Server) webLoginRevokeCLI(w http.ResponseWriter, r *http.Request) {
	if s.web == nil {
		writeJSON(w, webOffLogins())
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad JSON", http.StatusBadRequest)
		return
	}
	switch err := s.revokeWebLogin(body.ID, "cli"); {
	case errors.Is(err, ErrWebLoginNotFound):
		s.logWebAuth("web login revoke refused", "cli", "", ReasonLoginNotFound)
		writeJSON(w, WebLoginsResult{Refused: true, Reason: ReasonLoginNotFound, Message: "no web login has that id"})
	case err != nil:
		s.log.Error("web revoke failed", "err", err)
		writeJSON(w, WebLoginsResult{Refused: true, Reason: ReasonWebRevokeFailed, Message: "could not revoke the web login: " + err.Error()})
	default:
		writeJSON(w, WebLoginsResult{})
	}
}

// webSetPasswordCLI saves a new password without asking for the old one, signs
// out every web login, ends their streams and logs a password change from "cli".
func (s *Server) webSetPasswordCLI(w http.ResponseWriter, r *http.Request) {
	if s.web == nil {
		writeJSON(w, webOffLogins())
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad JSON", http.StatusBadRequest)
		return
	}
	removed, err := s.webAuth.SetPassword(body.Password)
	switch {
	case errors.Is(err, ErrPasswordTooShort):
		s.logWebAuth("web password change refused", "cli", "", ReasonPasswordTooShort)
		writeJSON(w, WebLoginsResult{Refused: true, Reason: ReasonPasswordTooShort, Message: "the password is too short: use at least 12 characters"})
	case err != nil:
		s.log.Error("web password change failed", "err", err)
		writeJSON(w, WebLoginsResult{Refused: true, Reason: ReasonWebPasswordFailed, Message: "could not save the password: " + err.Error()})
	default:
		s.logWebAuth("web password changed", "cli", "", "")
		s.webStreams.closeLogins(removed...)
		writeJSON(w, WebLoginsResult{})
	}
}

func webOffInfo() WebInfo {
	return WebInfo{Refused: true, Reason: ReasonWebOff, Message: "the web UI is off: set server.web.listen in the config and restart the server"}
}

func (s *Server) webInfoNow() WebInfo {
	if s.web == nil {
		return webOffInfo()
	}
	skipped := s.webCerts.Skipped()
	info := WebInfo{
		BaseURL:      s.WebURL(),
		Fingerprint:  webRootFingerprint(s.webCerts.Root().Raw),
		LeftOutHosts: skipped.Hosts,
	}
	pending, code, expires, err := s.webAuth.SetupStateWithExpiry()
	if err != nil {
		s.log.Error("web setup state unavailable", "err", err)
	}
	info.SetupPending, info.SetupCode, info.SetupExpires = pending, code, expires
	if !pending && s.webTOTPOn() {
		s.addResetCode(&info)
	}
	for _, ip := range skipped.IPs {
		info.LeftOutIPs = append(info.LeftOutIPs, ip.String())
	}
	return info
}
