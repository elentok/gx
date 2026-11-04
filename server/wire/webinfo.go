package wire

import "time"

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

// WebLogin is one signed-in browser, as shown in the device list.
type WebLogin struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UserAgent string    `json:"user_agent"`
	Created   time.Time `json:"created"`
	LastUsed  time.Time `json:"last_used"`
	LastIP    string    `json:"last_ip"`
}

// WebLoginsResult is the payload of the web login list and revoke routes behind
// `gx server web-logins` and `web-revoke`. A revoke answers with no Logins.
type WebLoginsResult struct {
	Logins  []WebLogin `json:"logins,omitempty"`
	Refused bool       `json:"refused,omitempty"`
	Reason  string     `json:"reason,omitempty"`
	Message string     `json:"message,omitempty"`
}

// Refusal and Refuse let the CLI treat both web payloads as one Refusable.
func (i WebInfo) Refusal() (bool, string, string) { return i.Refused, i.Reason, i.Message }
func (WebInfo) Refuse(reason, message string) WebInfo {
	return WebInfo{Refused: true, Reason: reason, Message: message}
}

func (r WebLoginsResult) Refusal() (bool, string, string) { return r.Refused, r.Reason, r.Message }
func (WebLoginsResult) Refuse(reason, message string) WebLoginsResult {
	return WebLoginsResult{Refused: true, Reason: reason, Message: message}
}

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
