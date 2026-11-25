package web

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/webperf"
)

const (
	webPerfSchemaVersion = 1
	webPerfMaxBody       = 32 << 10
	webPerfMaxSamples    = 50
	webPerfRateLimit     = 30
	webPerfWindow        = time.Minute
	webPerfMaxString     = 64
	webPerfMaxSkew       = 24 * time.Hour
)

var (
	webPerfMetrics = []string{"lcp", "inp", "cls", "fcp", "app_ready", "route_ready"}
	webPerfRoutes  = []string{"epics", "epic", "archive", "archive-epic", "agents", "stats", "cleanup", "schedules", "schedule", "tips", "settings", "design", "triage", "ticket", "answer"}
	webPerfEnums   = map[string][]string{
		"outcome":          {"ready", "error", "empty"},
		"trigger":          {"click", "keyboard", "history", "programmatic"},
		"data_source":      {"cache", "network"},
		"env":              {"prod", "dev"},
		"navigation_type":  {"navigate", "reload", "back-forward", "back-forward-cache"},
		"tab":              {"hard", "route"},
		"interaction_type": {"pointer", "keyboard"},
	}
	// token is the only shape an id, build or action name may take: no slash,
	// colon, space, query or hash, so no URL, address or selector fits.
	webPerfToken = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

// webPerfBody is the request of POST /v1/web-performance.
//
//	{"v": 1, "session": "...", "build_id": "...", "web_version": "...",
//	 "samples": [{"id": "...", "metric": "lcp", "value": 1234.5, "ts": 1700000000000, ...}]}
//
// Every sample needs id (unique within the session), metric (lcp, inp, cls,
// fcp, app_ready, route_ready), value (a finite number, ms except cls) and ts
// (browser time in unix ms, within a day of the server). Optional per sample:
// route (a normalized route name), env (prod, dev), navigation_type (navigate,
// reload, back-forward, back-forward-cache), tab (hard, route), outcome (ready,
// error, empty), trigger (click, keyboard, history, programmatic), data_source
// (cache, network), and the diagnostics ttfb, api_latency, js_bytes, css_bytes.
// A poor inp (over 200 ms) or poor lcp (over 2500 ms) may also carry
// attribution: input_delay, processing, presentation_delay, interaction_type,
// action, worst_chunk, worst_frame, lcp_load_delay, lcp_load_duration,
// lcp_render_delay. Any other sample has its attribution dropped.
//
// The answer is 200 {"stored": n, "dropped": n}, or 200 {"refused": true,
// "reason": "too-large"|"rate-limited"} with nothing stored. Bad JSON or a
// schema version other than 1 is 400.
type webPerfBody struct {
	Version    int             `json:"v"`
	Session    string          `json:"session"`
	BuildID    string          `json:"build_id"`
	WebVersion string          `json:"web_version"`
	Samples    []webPerfSample `json:"samples"`
}

type webPerfSample struct {
	ID             string   `json:"id"`
	Metric         string   `json:"metric"`
	Value          *float64 `json:"value"`
	Timestamp      int64    `json:"ts"`
	Route          string   `json:"route"`
	Env            string   `json:"env"`
	NavigationType string   `json:"navigation_type"`
	Tab            string   `json:"tab"`
	Outcome        string   `json:"outcome"`
	Trigger        string   `json:"trigger"`
	DataSource     string   `json:"data_source"`

	TTFB       *float64 `json:"ttfb"`
	APILatency *float64 `json:"api_latency"`
	JSBytes    *float64 `json:"js_bytes"`
	CSSBytes   *float64 `json:"css_bytes"`

	InputDelay        *float64 `json:"input_delay"`
	Processing        *float64 `json:"processing"`
	PresentationDelay *float64 `json:"presentation_delay"`
	InteractionType   string   `json:"interaction_type"`
	Action            string   `json:"action"`
	WorstChunk        string   `json:"worst_chunk"`
	WorstFrame        *float64 `json:"worst_frame"`
	LCPLoadDelay      *float64 `json:"lcp_load_delay"`
	LCPLoadDuration   *float64 `json:"lcp_load_duration"`
	LCPRenderDelay    *float64 `json:"lcp_render_delay"`
}

type webPerfResult struct {
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Stored  int    `json:"stored"`
	Dropped int    `json:"dropped"`
}

func (w *Web) webPerfReport(rw http.ResponseWriter, r *http.Request) {
	refuse := func(reason string) { wire.WriteJSON(rw, webPerfResult{Refused: true, Reason: reason}) }
	client := webauth.LoginIDFrom(r.Context()) + "|" + webauth.RemoteIP(r)
	if !w.perfLimit.allow(client, webPerfRateLimit, webPerfWindow) {
		refuse("rate-limited")
		return
	}
	var body webPerfBody
	r.Body = http.MaxBytesReader(rw, r.Body, webPerfMaxBody)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			refuse("too-large")
			return
		}
		http.Error(rw, "bad JSON", http.StatusBadRequest)
		return
	}
	if body.Version != webPerfSchemaVersion {
		http.Error(rw, "unsupported schema version", http.StatusBadRequest)
		return
	}
	if len(body.Samples) > webPerfMaxSamples {
		refuse("too-large")
		return
	}
	received := w.now()
	var valid []webperf.Sample
	for i := range body.Samples {
		if sm, ok := body.Samples[i].convert(&body, received); ok {
			valid = append(valid, sm)
		}
	}
	stored, dropped, err := w.host.WebPerf().Append(received, valid)
	if err != nil {
		w.log.Warn("web performance store failed", "err", err)
		http.Error(rw, "store failed", http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(rw, webPerfResult{Stored: stored, Dropped: dropped + len(body.Samples) - len(valid)})
}

func finite(p *float64) bool {
	return p == nil || (!math.IsNaN(*p) && !math.IsInf(*p, 0) && *p >= 0)
}

func goodToken(s string, required bool) bool {
	if s == "" {
		return !required
	}
	return len(s) <= webPerfMaxString && webPerfToken.MatchString(s)
}

// goodVersion is looser than goodToken: a `git describe` version may hold `+`
// or `/`, and dropping every sample of that build would lose its data.
func goodVersion(s string) bool {
	return len(s) <= webPerfMaxString && !strings.ContainsFunc(s, unicode.IsControl)
}

// convert validates one sample and returns the stored form. Attribution is
// kept per metric: a poor inp keeps the inp fields, a poor lcp the lcp fields.
func (s *webPerfSample) convert(b *webPerfBody, received time.Time) (webperf.Sample, bool) {
	enumOK := func(name, v string) bool { return v == "" || slices.Contains(webPerfEnums[name], v) }
	if !slices.Contains(webPerfMetrics, s.Metric) || s.Value == nil || !finite(s.Value) ||
		(s.Route != "" && !slices.Contains(webPerfRoutes, s.Route)) ||
		!enumOK("outcome", s.Outcome) || !enumOK("trigger", s.Trigger) || !enumOK("data_source", s.DataSource) ||
		!enumOK("env", s.Env) || !enumOK("navigation_type", s.NavigationType) || !enumOK("tab", s.Tab) ||
		!enumOK("interaction_type", s.InteractionType) ||
		!goodToken(b.Session, true) || !goodToken(s.ID, true) || !goodToken(b.BuildID, false) || !goodVersion(b.WebVersion) ||
		!goodToken(s.Action, false) || !goodToken(s.WorstChunk, false) {
		return webperf.Sample{}, false
	}
	for _, p := range []*float64{s.TTFB, s.APILatency, s.JSBytes, s.CSSBytes, s.InputDelay, s.Processing, s.PresentationDelay, s.WorstFrame, s.LCPLoadDelay, s.LCPLoadDuration, s.LCPRenderDelay} {
		if !finite(p) {
			return webperf.Sample{}, false
		}
	}
	if d := received.Sub(time.UnixMilli(s.Timestamp)); s.Timestamp == 0 || d > webPerfMaxSkew || d < -webPerfMaxSkew {
		return webperf.Sample{}, false
	}
	sm := webperf.Sample{
		ID: s.ID, Session: b.Session, Metric: s.Metric, Value: *s.Value, Route: s.Route,
		BuildID: b.BuildID, WebVersion: b.WebVersion, Env: s.Env, NavigationType: s.NavigationType,
		Tab: s.Tab, Timestamp: s.Timestamp, Outcome: s.Outcome, Trigger: s.Trigger, DataSource: s.DataSource,
		TTFB: s.TTFB, APILatency: s.APILatency, JSBytes: s.JSBytes, CSSBytes: s.CSSBytes,
	}
	switch {
	case s.Metric == "inp" && *s.Value > webperf.PoorINP:
		sm.InputDelay, sm.Processing, sm.PresentationDelay = s.InputDelay, s.Processing, s.PresentationDelay
		sm.InteractionType, sm.Action, sm.WorstChunk, sm.WorstFrame = s.InteractionType, s.Action, s.WorstChunk, s.WorstFrame
	case s.Metric == "lcp" && *s.Value > webperf.PoorLCP:
		sm.LCPLoadDelay, sm.LCPLoadDuration, sm.LCPRenderDelay = s.LCPLoadDelay, s.LCPLoadDuration, s.LCPRenderDelay
	}
	return sm, true
}
