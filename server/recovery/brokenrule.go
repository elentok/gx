package recovery

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/server/wire/ops"
)

// webUISource names where a broken rule report's repeats are seen.
const webUISource = "the web UI"

// FileBrokenRule files a draft follow-up for a broken rule the web UI reported,
// or counts a repeat on the open draft of the same rule and value. An
// unregistered target project only logs: the browser has nothing to retry.
func (rec *Recovery) FileBrokenRule(r ops.BrokenRuleReport) error {
	// A bare epic name in the setting means that epic in gx, whose web UI this is.
	epicPath, err := rec.FollowUpsEpicPath("gx")
	if errors.Is(err, errNoFollowUpProject) {
		rec.d.Log.Warn("follow-ups project missing, broken rule only logged", "rule", r.Rule, "err", err)
		return nil
	}
	if err != nil {
		return err
	}
	_, err = WriteFollowUpIn(epicPath, webUISource, brokenRuleFollowUp(r))
	return err
}

func brokenRuleFollowUp(r ops.BrokenRuleReport) ticketwrite.FollowUp {
	return ticketwrite.FollowUp{
		Slug:   "broken-rule-" + r.Rule,
		Title:  "Broken rule in the web UI: " + r.Rule,
		Source: "A broken rule report from " + webUISource + ".",
		Problem: strings.Join([]string{
			OutsideTextLine("rule", r.Rule, 64),
			OutsideTextLine("message", r.Message, 500),
			OutsideTextLine("value", r.Value, 200),
			OutsideTextLine("page URL", r.URL, 500),
			OutsideTextLine("user agent", r.UserAgent, 300),
		}, "\n"),
		SuggestedFix: "Find where the web UI breaks the rule named in the Problem section and fix it.",
		// The hash covers the whole value, so cutting a long one never merges two.
		Key: fmt.Sprintf("broken-rule/%s/%x", r.Rule, sha256.Sum256([]byte(r.Value))),
	}
}
