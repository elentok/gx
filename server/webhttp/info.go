package webhttp

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/server/webtls"
	"github.com/elentok/gx/server/wire"
)

// Info answers the CLI with the web UI's address, fingerprint and Setup code.
func (w *Web) Info(rw http.ResponseWriter, _ *http.Request) {
	writeJSON(rw, w.info())
}

// NewRoot makes a new root from the current allowed hosts and swaps root and
// leaf live. A bad Host header or token is turned away before this runs.
func (w *Web) NewRoot(rw http.ResponseWriter, _ *http.Request) {
	if w.ln == nil {
		writeJSON(rw, offInfo())
		return
	}
	if err := w.certs.NewRoot(); err != nil {
		w.log.Error("new root failed", "err", err)
		writeJSON(rw, wire.WebInfo{Refused: true, Reason: wire.ReasonWebNewRootFailed, Message: "could not make a new root: " + err.Error()})
		return
	}
	info := w.info()
	w.log.Info("new root", "fingerprint", info.Fingerprint)
	writeJSON(rw, info)
}

// Reset clears the web account, ends every web login's event streams and
// answers with the new Setup code. The socket is the only way in.
func (w *Web) Reset(rw http.ResponseWriter, _ *http.Request) {
	if w.ln == nil {
		writeJSON(rw, offInfo())
		return
	}
	removed, err := w.auth.WebReset()
	if err != nil {
		w.log.Error("web reset failed", "err", err)
		writeJSON(rw, wire.WebInfo{Refused: true, Reason: wire.ReasonWebResetFailed, Message: "could not reset the web account: " + err.Error()})
		return
	}
	w.auth.DropLogins(removed...)
	w.auth.LogEvent("web reset", "cli", "", "")
	writeJSON(rw, w.info())
}

// ResetAuthenticator is the CLI authenticator reset: it signs out every web
// login, ends their streams and answers with the new Setup code. The answer
// holds no TOTP secret.
func (w *Web) ResetAuthenticator(rw http.ResponseWriter, _ *http.Request) {
	if w.ln == nil {
		writeJSON(rw, offInfo())
		return
	}
	removed, _, err := w.auth.ClearAuthenticator()
	if err != nil {
		w.log.Error("web authenticator reset failed", "err", err)
		writeJSON(rw, wire.WebInfo{Refused: true, Reason: wire.ReasonWebAuthenticatorResetFailed, Message: "could not reset the authenticator: " + err.Error()})
		return
	}
	w.auth.DropLogins(removed...)
	w.auth.LogEvent("web authenticator reset", "cli", "", "")
	info := w.info()
	w.addResetCode(&info)
	writeJSON(rw, info)
}

// addResetCode puts the pending authenticator reset's Setup code on info.
func (w *Web) addResetCode(info *wire.WebInfo) {
	code, expires, ok, err := w.auth.AuthenticatorResetCode()
	if err != nil {
		w.log.Error("web setup code unavailable", "err", err)
		return
	}
	if ok {
		info.SetupCode, info.SetupExpires = code, expires
	}
}

func offLogins() wire.WebLoginsResult {
	off := offInfo()
	return wire.WebLoginsResult{Refused: true, Reason: off.Reason, Message: off.Message}
}

// LoginList answers the CLI with every web login.
func (w *Web) LoginList(rw http.ResponseWriter, _ *http.Request) {
	if w.ln == nil {
		writeJSON(rw, offLogins())
		return
	}
	writeJSON(rw, wire.WebLoginsResult{Logins: w.auth.List()})
}

// LoginRevoke revokes one web login by id. An unknown id is a refusal.
func (w *Web) LoginRevoke(rw http.ResponseWriter, r *http.Request) {
	if w.ln == nil {
		writeJSON(rw, offLogins())
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(rw, "bad JSON", http.StatusBadRequest)
		return
	}
	switch err := w.auth.Revoke(body.ID, "cli"); {
	case errors.Is(err, webauth.ErrWebLoginNotFound):
		w.auth.LogEvent("web login revoke refused", "cli", "", wire.ReasonLoginNotFound)
		writeJSON(rw, wire.WebLoginsResult{Refused: true, Reason: wire.ReasonLoginNotFound, Message: "no web login has that id"})
	case err != nil:
		w.log.Error("web revoke failed", "err", err)
		writeJSON(rw, wire.WebLoginsResult{Refused: true, Reason: wire.ReasonWebRevokeFailed, Message: "could not revoke the web login: " + err.Error()})
	default:
		writeJSON(rw, wire.WebLoginsResult{})
	}
}

// SetPassword saves a new password without asking for the old one, signs out
// every web login, ends their streams and logs a password change from "cli".
func (w *Web) SetPassword(rw http.ResponseWriter, r *http.Request) {
	if w.ln == nil {
		writeJSON(rw, offLogins())
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(rw, "bad JSON", http.StatusBadRequest)
		return
	}
	removed, err := w.auth.SetPassword(body.Password)
	switch {
	case errors.Is(err, webauth.ErrPasswordTooShort):
		w.auth.LogEvent("web password change refused", "cli", "", wire.ReasonPasswordTooShort)
		writeJSON(rw, wire.WebLoginsResult{Refused: true, Reason: wire.ReasonPasswordTooShort, Message: "the password is too short: use at least 6 characters"})
	case err != nil:
		w.log.Error("web password change failed", "err", err)
		writeJSON(rw, wire.WebLoginsResult{Refused: true, Reason: wire.ReasonWebPasswordFailed, Message: "could not save the password: " + err.Error()})
	default:
		w.auth.LogEvent("web password changed", "cli", "", "")
		w.auth.DropLogins(removed...)
		writeJSON(rw, wire.WebLoginsResult{})
	}
}

func offInfo() wire.WebInfo {
	return wire.WebInfo{Refused: true, Reason: wire.ReasonWebOff, Message: "the web UI is off: set server.web.listen in the config and restart the server"}
}

func (w *Web) info() wire.WebInfo {
	if w.ln == nil {
		return offInfo()
	}
	skipped := w.certs.Skipped()
	info := wire.WebInfo{
		BaseURL:      w.URL(),
		Fingerprint:  webtls.RootFingerprint(w.certs.Root().Raw),
		LeftOutHosts: skipped.Hosts,
	}
	pending, code, expires, err := w.auth.SetupStateWithExpiry()
	if err != nil {
		w.log.Error("web setup state unavailable", "err", err)
	}
	info.SetupPending, info.SetupCode, info.SetupExpires = pending, code, expires
	if !pending && w.auth.TOTPOn() {
		w.addResetCode(&info)
	}
	for _, ip := range skipped.IPs {
		info.LeftOutIPs = append(info.LeftOutIPs, ip.String())
	}
	return info
}
