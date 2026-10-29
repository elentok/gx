package server

import "net/http"

// ReasonWebOff refuses a web read when the web listener is off or failed to bind.
const ReasonWebOff = "web-off"

// WebInfo is the GET /v1/web/info payload behind `gx server web-url`.
type WebInfo struct {
	BaseURL string `json:"base_url,omitempty"`
	// Fingerprint is the SHA-256 of the root certificate, in 8 groups of 8.
	Fingerprint string `json:"fingerprint,omitempty"`
	// LeftOutHosts and LeftOutIPs are the names the leaf omits because the root does not permit them.
	LeftOutHosts []string `json:"left_out_hosts,omitempty"`
	LeftOutIPs   []string `json:"left_out_ips,omitempty"`
	Refused      bool     `json:"refused,omitempty"`
	Reason       string   `json:"reason,omitempty"`
	Message      string   `json:"message,omitempty"`
}

func (s *Server) webInfo(w http.ResponseWriter, _ *http.Request) {
	if s.web == nil {
		writeJSON(w, WebInfo{Refused: true, Reason: ReasonWebOff, Message: "the web UI is off: set server.web.listen in the config and restart the server"})
		return
	}
	skipped := s.webCerts.Skipped()
	info := WebInfo{
		BaseURL:      s.WebURL(),
		Fingerprint:  webRootFingerprint(s.webRoot.Raw),
		LeftOutHosts: skipped.Hosts,
	}
	for _, ip := range skipped.IPs {
		info.LeftOutIPs = append(info.LeftOutIPs, ip.String())
	}
	writeJSON(w, info)
}
