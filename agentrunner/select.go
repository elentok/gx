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
	// ChoicePTY is what auto picks when claude is on PATH.
	ChoicePTY Choice = "pty"
)

// HerdrPingTimeout bounds how long an explicit herdr choice waits for herdr.
const HerdrPingTimeout = 2 * time.Second

// DefaultN is the runner wait N when the config does not set one. It bounds
// the busy guard's wait, the `/` rule's idle-after-N, Interrupt's wait for idle
// and herdr's deny wait.
const DefaultN = 5 * time.Second

// Probe is what Select needs to know about the machine.
type Probe struct {
	LookPath func(file string) (string, error)
	// PingHerdr reports whether herdr answers within timeout.
	PingHerdr func(timeout time.Duration) error
}

// Select resolves setting to the runner to use. Auto picks PTY and never pings
// herdr. An explicit herdr with herdr down is an error, not a fallback.
func Select(setting string, p Probe) (Choice, error) {
	if err := ValidateSetting(setting); err != nil {
		return "", err
	}
	c := Choice(setting)
	switch c {
	case "", ChoiceAuto:
		if _, err := p.LookPath("claude"); err != nil {
			return "", fmt.Errorf("agent-runner auto needs claude on PATH (or set agent-runner: herdr): %w", err)
		}
		return ChoicePTY, nil
	case ChoiceHerdr:
		if err := herdrUp(p); err != nil {
			return "", fmt.Errorf("agent-runner is herdr but herdr is unavailable: %w", err)
		}
		return ChoiceHerdr, nil
	}
	if _, err := p.LookPath("claude"); err != nil {
		return "", fmt.Errorf("agent-runner %s needs claude on PATH: %w", c, err)
	}
	return c, nil
}

// ValidateSetting checks setting against the allowed values without probing
// the machine.
func ValidateSetting(setting string) error {
	switch Choice(setting) {
	case "", ChoiceAuto, ChoiceHerdr, ChoiceHeadless, ChoicePTY:
		return nil
	default:
		return fmt.Errorf("invalid agent-runner %q: want auto, herdr, headless or pty", setting)
	}
}

func herdrUp(p Probe) error {
	if _, err := p.LookPath("herdr"); err != nil {
		return err
	}
	return p.PingHerdr(HerdrPingTimeout)
}
