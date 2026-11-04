package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/server/webtls"
	"github.com/elentok/gx/server/wire"
)

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
	if err := s.webCerts.NewRoot(); err != nil {
		s.log.Error("new root failed", "err", err)
		writeJSON(w, wire.WebInfo{Refused: true, Reason: wire.ReasonWebNewRootFailed, Message: "could not make a new root: " + err.Error()})
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
		writeJSON(w, wire.WebInfo{Refused: true, Reason: wire.ReasonWebResetFailed, Message: "could not reset the web account: " + err.Error()})
		return
	}
	s.webAuth.DropLogins(removed...)
	s.webAuth.LogEvent("web reset", "cli", "", "")
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
		writeJSON(w, wire.WebInfo{Refused: true, Reason: wire.ReasonWebAuthenticatorResetFailed, Message: "could not reset the authenticator: " + err.Error()})
		return
	}
	s.webAuth.DropLogins(removed...)
	s.webAuth.LogEvent("web authenticator reset", "cli", "", "")
	info := s.webInfoNow()
	s.addResetCode(&info)
	writeJSON(w, info)
}

// addResetCode puts the pending authenticator reset's Setup code on info.
func (s *Server) addResetCode(info *wire.WebInfo) {
	code, expires, ok, err := s.webAuth.AuthenticatorResetCode()
	if err != nil {
		s.log.Error("web setup code unavailable", "err", err)
		return
	}
	if ok {
		info.SetupCode, info.SetupExpires = code, expires
	}
}

func webOffLogins() wire.WebLoginsResult {
	off := webOffInfo()
	return wire.WebLoginsResult{Refused: true, Reason: off.Reason, Message: off.Message}
}

// webLoginListCLI answers the CLI with every web login.
func (s *Server) webLoginListCLI(w http.ResponseWriter, _ *http.Request) {
	if s.web == nil {
		writeJSON(w, webOffLogins())
		return
	}
	writeJSON(w, wire.WebLoginsResult{Logins: s.webAuth.List()})
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
	switch err := s.webAuth.Revoke(body.ID, "cli"); {
	case errors.Is(err, webauth.ErrWebLoginNotFound):
		s.webAuth.LogEvent("web login revoke refused", "cli", "", wire.ReasonLoginNotFound)
		writeJSON(w, wire.WebLoginsResult{Refused: true, Reason: wire.ReasonLoginNotFound, Message: "no web login has that id"})
	case err != nil:
		s.log.Error("web revoke failed", "err", err)
		writeJSON(w, wire.WebLoginsResult{Refused: true, Reason: wire.ReasonWebRevokeFailed, Message: "could not revoke the web login: " + err.Error()})
	default:
		writeJSON(w, wire.WebLoginsResult{})
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
	case errors.Is(err, webauth.ErrPasswordTooShort):
		s.webAuth.LogEvent("web password change refused", "cli", "", wire.ReasonPasswordTooShort)
		writeJSON(w, wire.WebLoginsResult{Refused: true, Reason: wire.ReasonPasswordTooShort, Message: "the password is too short: use at least 6 characters"})
	case err != nil:
		s.log.Error("web password change failed", "err", err)
		writeJSON(w, wire.WebLoginsResult{Refused: true, Reason: wire.ReasonWebPasswordFailed, Message: "could not save the password: " + err.Error()})
	default:
		s.webAuth.LogEvent("web password changed", "cli", "", "")
		s.webAuth.DropLogins(removed...)
		writeJSON(w, wire.WebLoginsResult{})
	}
}

func webOffInfo() wire.WebInfo {
	return wire.WebInfo{Refused: true, Reason: wire.ReasonWebOff, Message: "the web UI is off: set server.web.listen in the config and restart the server"}
}

func (s *Server) webInfoNow() wire.WebInfo {
	if s.web == nil {
		return webOffInfo()
	}
	skipped := s.webCerts.Skipped()
	info := wire.WebInfo{
		BaseURL:      s.WebURL(),
		Fingerprint:  webtls.RootFingerprint(s.webCerts.Root().Raw),
		LeftOutHosts: skipped.Hosts,
	}
	pending, code, expires, err := s.webAuth.SetupStateWithExpiry()
	if err != nil {
		s.log.Error("web setup state unavailable", "err", err)
	}
	info.SetupPending, info.SetupCode, info.SetupExpires = pending, code, expires
	if !pending && s.webAuth.TOTPOn() {
		s.addResetCode(&info)
	}
	for _, ip := range skipped.IPs {
		info.LeftOutIPs = append(info.LeftOutIPs, ip.String())
	}
	return info
}
