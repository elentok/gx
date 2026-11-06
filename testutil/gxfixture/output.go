package main

import (
	"fmt"
	"strings"
)

// Output is the JSON gxfixture prints once the server answers.
type Output struct {
	// WebURL is the plain URL: it signs nobody in. Cookie is the web login's
	// cookie value under CookieName, from the setup the fixture finished; it is
	// empty in the web-setup and web-login scenarios, where the page starts signed out.
	// Password is the web password, empty while the setup is pending.
	WebURL     string `json:"web_url"`
	CookieName string `json:"cookie_name"`
	Cookie     string `json:"cookie"`
	Password   string `json:"password,omitempty"`
	// TOTPSecret and TOTPStep (the last step used) are left out when the server
	// runs with server.web.totp=false.
	TOTPSecret string `json:"totp_secret,omitempty"`
	TOTPStep   int64  `json:"totp_step,omitempty"`
	StateDir   string `json:"state_dir"`
	Socket     string `json:"socket"`
	HoldFile   string `json:"hold_file"`
	// Env is what a CLI verb needs to reach this server and no other.
	Env []string `json:"env"`
	// Gx is the gx binary the server runs.
	Gx string `json:"gx"`
	// DiskDir holds the fake disk's control files: free and clean.
	DiskDir string `json:"disk_dir"`
	// HealthDir holds the fake CPU and memory control file: system.
	HealthDir string `json:"health_dir"`
	// GateDir holds the cache-gate scenarios' fake go (bin/go) and its control files; empty elsewhere.
	// The launch-held scenario also uses it: bin/npm waits while npm-hold exists, and npm-started marks the wait.
	GateDir string `json:"gate_dir,omitempty"`
	// ConfigFile is the global config file; edit it and run `gx server reload-config` to change a setting live.
	ConfigFile string `json:"config_file"`
	// ChatCapture is the file the chat-capture scenario's endpoint appends every Slack and
	// Telegram post to, one JSON line each; empty elsewhere.
	ChatCapture string `json:"chat_capture,omitempty"`
	// RestartDir holds the fake restart binary's control files; empty outside the restart scenarios.
	RestartDir string `json:"restart_dir,omitempty"`
	// MigrateDir holds the migrate scenarios' control files and server script (see migrate.go); empty elsewhere.
	MigrateDir string `json:"migrate_dir,omitempty"`
	// LogFile is the server's combined stdout and stderr, which carries the cleanup
	// command's output tail ("disk cleanup command failed") and every server warning.
	LogFile string `json:"log_file"`
	// AgentTranscript is the held fake agent's transcript path in the spend
	// scenario; the directory may not exist yet.
	AgentTranscript string `json:"agent_transcript,omitempty"`
	// CheckWaitingFile is the live check-lock waiting state of the check-lock
	// scenario; removing it ends the wait. Empty elsewhere.
	CheckWaitingFile string `json:"check_waiting_file,omitempty"`
	// Transcripts are the limit-burn live shape's Claude transcript paths, keyed
	// queue, one_off, design and other; the directories may not exist yet.
	Transcripts map[string]string `json:"transcripts,omitempty"`
	// CodexSessionsDir holds the held Codex worker's rollout in the limit-burn
	// scenario; empty elsewhere.
	CodexSessionsDir string `json:"codex_sessions_dir,omitempty"`
	// MockURL is the base URL serving --mock-dir; empty without the flag.
	MockURL string `json:"mock_url,omitempty"`
	// BrowserSession names this worktree's playwright-cli session (`-s=<name>`): the
	// worktree directory is unique per sibling session and stable across calls, so
	// concurrent sessions never share a browser.
	BrowserSession string `json:"browser_session,omitempty"`
	// FeatureWorktree is demo's `feature` worktree, in scenarios that make one.
	FeatureWorktree string `json:"feature_worktree,omitempty"`
}

// ShellEnv is the output as `export` lines a shell can source: the CLI env, then
// GXF_* variables for the rest. Values are single-quoted.
func (o Output) ShellEnv() string {
	var b strings.Builder
	export := func(k, v string) { fmt.Fprintf(&b, "export %s='%s'\n", k, strings.ReplaceAll(v, "'", `'\''`)) }
	for _, kv := range o.Env {
		k, v, _ := strings.Cut(kv, "=")
		export(k, v)
	}
	export("GXF_WEB_URL", o.WebURL)
	export("GXF_WEB_COOKIE", o.CookieName+"="+o.Cookie)
	export("GXF_GX", o.Gx)
	export("GXF_DISK_DIR", o.DiskDir)
	export("GXF_HEALTH_DIR", o.HealthDir)
	export("GXF_HOLD_FILE", o.HoldFile)
	export("GXF_STATE_DIR", o.StateDir)
	export("GXF_SOCKET", o.Socket)
	if o.GateDir != "" {
		export("GXF_GATE_DIR", o.GateDir)
	}
	if o.ConfigFile != "" {
		export("GXF_CONFIG_FILE", o.ConfigFile)
	}
	if o.RestartDir != "" {
		export("GXF_RESTART_DIR", o.RestartDir)
	}
	if o.MigrateDir != "" {
		export("GXF_MIGRATE_DIR", o.MigrateDir)
	}
	if o.LogFile != "" {
		export("GXF_LOG_FILE", o.LogFile)
	}
	if o.AgentTranscript != "" {
		export("GXF_AGENT_TRANSCRIPT", o.AgentTranscript)
	}
	if o.MockURL != "" {
		export("GXF_MOCK_URL", o.MockURL)
	}
	if o.BrowserSession != "" {
		export("GXF_BROWSER_SESSION", o.BrowserSession)
	}
	if o.FeatureWorktree != "" {
		export("GXF_FEATURE_WORKTREE", o.FeatureWorktree)
	}
	return b.String()
}
