package agentrunner

import (
	"fmt"
	"time"
)

// Choice is a value of the per-machine agent-runner setting.
type Choice string

const (
	ChoiceAuto     Choice = "auto"
	ChoiceHerdr    Choice = "herdr"
	ChoiceHeadless Choice = "headless"
	ChoicePTY      Choice = "pty"
)

// HerdrPingTimeout bounds how long auto waits for herdr before it falls back
// to headless.
const HerdrPingTimeout = 2 * time.Second

// Probe is what Select needs to know about the machine.
type Probe struct {
	LookPath func(file string) (string, error)
	// PingHerdr reports whether herdr answers within timeout.
	PingHerdr func(timeout time.Duration) error
}

// Select resolves setting to the runner to use. auto never picks pty: a
// blocked `claude -p` is too hard to detect, so pty is only ever chosen by
// hand. An explicit herdr with herdr down is an error, not a fallback.
func Select(setting string, p Probe) (Choice, error) {
	c := Choice(setting)
	switch c {
	case "", ChoiceAuto:
		if herdrUp(p) == nil {
			return ChoiceHerdr, nil
		}
		c = ChoiceHeadless
	case ChoiceHerdr:
		if err := herdrUp(p); err != nil {
			return "", fmt.Errorf("agent-runner is herdr but herdr is unavailable: %w", err)
		}
		return ChoiceHerdr, nil
	case ChoiceHeadless, ChoicePTY:
	default:
		return "", fmt.Errorf("invalid agent-runner %q: want auto, herdr, headless or pty", setting)
	}
	if _, err := p.LookPath("claude"); err != nil {
		return "", fmt.Errorf("agent-runner %s needs claude on PATH: %w", c, err)
	}
	return c, nil
}

func herdrUp(p Probe) error {
	if _, err := p.LookPath("herdr"); err != nil {
		return err
	}
	return p.PingHerdr(HerdrPingTimeout)
}
