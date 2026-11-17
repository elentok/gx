package agentrunner

import (
	"fmt"
	"time"
)

// Choice is a value of the per-machine agent-runner setting.
type Choice string

const (
	ChoiceAuto     Choice = "auto"
	ChoiceHeadless Choice = "headless"
	// ChoicePTY is what auto picks when claude is on PATH.
	ChoicePTY Choice = "pty"
)

// HerdrPingTimeout bounds how long the TUI's multiplexer gate waits for herdr.
const HerdrPingTimeout = 2 * time.Second

// DefaultN is the runner wait N when the config does not set one. It bounds
// the busy guard's wait, the `/` rule's idle-after-N and Interrupt's wait for
// idle.
const DefaultN = 5 * time.Second

// Probe is what Select needs to know about the machine.
type Probe struct {
	LookPath func(file string) (string, error)
	// PingHerdr reports whether herdr answers within timeout.
	PingHerdr func(timeout time.Duration) error
}

// CheckHarness reports whether the program of harness ("claude" or "codex";
// empty means claude) is on PATH. The error names the program.
func CheckHarness(p Probe, harness string) error {
	if harness == "" {
		harness = "claude"
	}
	if _, err := p.LookPath(harness); err != nil {
		return fmt.Errorf("%s is not on PATH", harness)
	}
	return nil
}

// checkDefaultHarness is the auto/pty requirement: the Default harness must be
// on PATH.
func checkDefaultHarness(p Probe, harness string) error {
	err := CheckHarness(p, harness)
	if err == nil {
		return nil
	}
	if harness == "codex" {
		return fmt.Errorf("the default harness %q is not on PATH. Install codex or set the default to claude in ~/.config/gx/config.json.", harness)
	}
	return err
}

// Select resolves setting to the runner to use. Auto and pty need the Default
// harness on PATH; headless hosts only claude. Auto picks PTY and never pings
// herdr.
func Select(setting, defaultHarness string, p Probe) (Choice, error) {
	setting, _ = Normalise(setting)
	if err := ValidateSetting(setting); err != nil {
		return "", err
	}
	c := Choice(setting)
	switch c {
	case "", ChoiceAuto:
		if err := checkDefaultHarness(p, defaultHarness); err != nil {
			return "", err
		}
		return ChoicePTY, nil
	}
	if c == ChoicePTY {
		if err := checkDefaultHarness(p, defaultHarness); err != nil {
			return "", err
		}
		return c, nil
	}
	if err := CheckHarness(p, "claude"); err != nil {
		return "", fmt.Errorf("agent-runner %s needs claude: %w", c, err)
	}
	return c, nil
}

// ValidateSetting checks setting against the allowed values without probing
// the machine.
func ValidateSetting(setting string) error {
	setting, _ = Normalise(setting)
	switch Choice(setting) {
	case "", ChoiceAuto, ChoiceHeadless, ChoicePTY:
		return nil
	default:
		return fmt.Errorf("invalid agent-runner %q: want auto, headless or pty", setting)
	}
}
