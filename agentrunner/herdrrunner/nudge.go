package herdrrunner

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/herdr"
)

// promptWithNudge wraps herdr's AgentPrompt/AgentSendKeys/AgentWait/AgentRead
// in claudeui.PromptWithNudge. It first waits for the turn to start (either
// "working" or one of opts.Until, for a completion so fast "working" is never
// seen), then waits, without nudging, for opts.Until. Both phases share one
// deadline from opts.TimeoutMs, so a slow completion like "/compact" gets
// what's left of it rather than the grace window. opts.TimeoutMs <= 0 means
// no deadline: start waits stay bounded, completion waits unbounded. report
// may be nil.
func promptWithNudge(
	prompt func(herdr.AgentPromptOptions) (herdr.Agent, error),
	sendKeys func(target string, keys ...string) error,
	wait func(herdr.AgentWaitOptions) (herdr.Agent, error),
	read func(target string, opts herdr.AgentReadOptions) (string, error),
	now func() time.Time,
	report func(claudeui.NudgeDecision),
) func(herdr.AgentPromptOptions) (herdr.Agent, error) {
	return func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		if !opts.Wait {
			return prompt(opts)
		}

		var deadline time.Time
		if opts.TimeoutMs > 0 {
			deadline = now().Add(time.Duration(opts.TimeoutMs) * time.Millisecond)
		}

		p := &herdrPrompter{opts: opts, prompt: prompt, sendKeys: sendKeys, wait: wait, read: read}
		p.startUntil = opts.Until
		if !slices.Contains(p.startUntil, "working") {
			p.startUntil = append([]string{"working"}, p.startUntil...)
		}

		if err := claudeui.PromptWithNudge(p, opts.Text, claudeui.DefaultNudgeCaps, deadline, now, report); err != nil {
			return p.agent, p.wrapErr(err)
		}
		if slices.Contains(opts.Until, p.agent.AgentStatus) {
			return p.agent, nil
		}

		completionTimeoutMs := 0
		if !deadline.IsZero() {
			remaining := deadline.Sub(now())
			if remaining <= 0 {
				return p.agent, fmt.Errorf("timed out waiting for completion: overall deadline exceeded")
			}
			completionTimeoutMs = toMs(remaining)
		}
		return wait(herdr.AgentWaitOptions{Target: opts.Target, Until: opts.Until, TimeoutMs: completionTimeoutMs})
	}
}

// herdrPrompter is claudeui.Prompter over herdr. herdr types and waits in one
// AgentPrompt call, so Submit only marks the text pending and the next
// WaitStart sends it.
type herdrPrompter struct {
	opts       herdr.AgentPromptOptions
	startUntil []string
	prompt     func(herdr.AgentPromptOptions) (herdr.Agent, error)
	sendKeys   func(target string, keys ...string) error
	wait       func(herdr.AgentWaitOptions) (herdr.Agent, error)
	read       func(target string, opts herdr.AgentReadOptions) (string, error)

	pending bool
	agent   herdr.Agent
	pollErr error
}

func (p *herdrPrompter) Submit(text string) error {
	p.opts.Text = text
	p.pending = true
	return nil
}

// Clear presses Ctrl+U: unlike Esc or Ctrl+C it can't interrupt a turn that
// started unseen.
func (p *herdrPrompter) Clear() error {
	return p.sendKeys(p.opts.Target, "ctrl+u")
}

func (p *herdrPrompter) SendEnter() error {
	return p.sendKeys(p.opts.Target, "enter")
}

func (p *herdrPrompter) WaitStart(timeout time.Duration) (bool, error) {
	var err error
	if p.pending {
		p.pending = false
		submit := p.opts
		submit.Until = p.startUntil
		submit.TimeoutMs = toMs(timeout)
		p.agent, err = p.prompt(submit)
	} else {
		p.agent, err = p.wait(herdr.AgentWaitOptions{Target: p.opts.Target, Until: p.startUntil, TimeoutMs: toMs(timeout)})
	}
	if IsPollTimeout(err) {
		p.pollErr = err
		return false, nil
	}
	return err == nil, err
}

func (p *herdrPrompter) Screen() (string, error) {
	return p.read(p.opts.Target, herdr.AgentReadOptions{Source: "recent-unwrapped"})
}

// wrapErr keeps herdr's own poll-timeout text on a give-up, so callers still
// see why the last wait failed.
func (p *herdrPrompter) wrapErr(err error) error {
	if p.pollErr == nil {
		return err
	}
	if errors.Is(err, claudeui.ErrStartTimeout) {
		return p.pollErr
	}
	if errors.Is(err, claudeui.ErrStuckSubmission) {
		return fmt.Errorf("%w: %w", err, p.pollErr)
	}
	return err
}

func toMs(d time.Duration) int {
	return max(1, int(d/time.Millisecond))
}

// IsPollTimeout reports whether err looks like a transient herdr wait
// failure worth nudging and retrying rather than aborting: either AgentWait's
// own timeout-elapsed failure ("timed out waiting for agent status"), or
// herdr's agent_prompt_stalled error (the pane went idle with no observed
// state change within herdr's own internal stall window).
func IsPollTimeout(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timed out") ||
		strings.Contains(msg, "agent_prompt_stalled") ||
		strings.Contains(msg, "no observed state change")
}

// MatchedRuleID reads a pane's matched_rule.id via explain, falling back to
// "unknown" when explain is nil, fails, or returns no rule id.
func MatchedRuleID(explain func(string) (herdr.AgentExplainResult, error), pane string) string {
	if explain == nil {
		return "unknown"
	}
	if res, err := explain(pane); err == nil && res.MatchedRuleID != "" {
		return res.MatchedRuleID
	}
	return "unknown"
}
