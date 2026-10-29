package server

import (
	"net/http"
	"time"
)

// ReasonWebOff refuses a web read when the web listener is off or failed to bind.
const ReasonWebOff = "web-off"

// ReasonWebNewRootFailed refuses a new-root request when the root could not be made or saved.
const ReasonWebNewRootFailed = "web-new-root-failed"

// ReasonWebResetFailed refuses a web reset when the account could not be cleared or saved.
const ReasonWebResetFailed = "web-reset-failed"

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
	for _, ip := range skipped.IPs {
		info.LeftOutIPs = append(info.LeftOutIPs, ip.String())
	}
	return info
}
