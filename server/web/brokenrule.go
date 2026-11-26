package web

import (
	"net/http"
	"regexp"
	"slices"
	"time"

	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/server/wire"
)

const (
	brokenRuleMaxBody   = 16 << 10
	brokenRuleRateLimit = 20
	brokenRuleWindow    = time.Minute
	brokenRuleMaxID     = 64
)

var brokenRuleID = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// brokenRuleReport logs and files a rule the web UI found broken.
func (w *Web) brokenRuleReport(rw http.ResponseWriter, r *http.Request) {
	refuse := func(reason string) { wire.WriteJSON(rw, wire.BrokenRuleResult{Refused: true, Reason: reason}) }
	ip := webauth.RemoteIP(r)
	var report wire.BrokenRuleReport
	if !readReport(rw, r, w.rules.allow(ip, brokenRuleRateLimit, brokenRuleWindow), brokenRuleMaxBody, &report, refuse) {
		return
	}
	if slices.Contains([]string{report.Rule, report.Message, report.Value, report.URL, report.UserAgent}, "") {
		refuse("incomplete")
		return
	}
	if len(report.Rule) > brokenRuleMaxID || !brokenRuleID.MatchString(report.Rule) {
		refuse("bad-rule")
		return
	}
	report.URL = cleanPageURL(report.URL)
	w.log.Warn("broken rule", "rule", report.Rule, "value", report.Value, "message", report.Message, "page_url", report.URL, "user_agent", report.UserAgent, "ip", ip)
	if err := w.host.FileBrokenRule(report); err != nil {
		w.log.Warn("file broken rule", "rule", report.Rule, "err", err)
		http.Error(rw, "filing failed", http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(rw, wire.BrokenRuleResult{})
}
