package agentrunner

// HerdrRemovedWarning is the one warning for a config that still says
// agent-runner: herdr.
const HerdrRemovedWarning = "agent-runner: herdr was removed; using auto (PTY). Edit the config to drop it."

// Normalise maps the removed herdr setting to auto. deprecated is true when
// it did. Every reader of the agent-runner setting goes through it.
func Normalise(setting string) (normalised string, deprecated bool) {
	if Choice(setting) == ChoiceHerdr {
		return string(ChoiceAuto), true
	}
	return setting, false
}
