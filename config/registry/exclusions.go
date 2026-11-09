package registry

const (
	reasonTUIOnly = "TUI only, not shown on the web page"
	reasonInert   = "valid in config.json but read by nothing outside config/, so a web save would change nothing"
	reasonIgnored = "accepted in project.json, ignored"
)

// Exclusions lists config keys deliberately left out of the registry.
func Exclusions() []Exclusion {
	out := []Exclusion{
		{Key: Key("$schema"), Project: true, Reason: "not a setting"},
		{Key: Key("agents", "resume-on-start"), Reason: "legacy alias"},
		{Key: Key("notifications"), Project: true,
			Reason: "accepted in project.json, deliberately not editable on the web (secrets); ProjectFile still loads it and the page leaves an existing block untouched"},
	}
	for _, k := range []string{"restore-sessions-on-start", "banners", "sounds", "claude-args", "fix-agent-remote-control", "leader-key", "focus-sidebar-key"} {
		out = append(out, Exclusion{Key: Key("agents", k), Project: true, Reason: reasonIgnored})
	}
	// Only the TUI process reads these; the server hardcodes the skill names,
	// so a web save would show "Saved" and change nothing.
	for _, k := range []string{"implement", "code-review"} {
		out = append(out,
			Exclusion{Key: Key("skills", k), Reason: reasonInert},
			Exclusion{Key: Key("skills", k), Project: true, Reason: reasonInert})
	}
	for _, k := range []KeyPath{
		Key("execution-queue", "retry-storm-launches"), Key("execution-queue", "spin-cycles"),
		Key("execution-queue", "spin-window"), Key("budget", "notification-thresholds"),
		Key("evolve.deep-read-cap"),
	} {
		out = append(out, Exclusion{Key: k, Reason: reasonInert})
	}
	for _, k := range []KeyPath{
		Key("use-nerdfont-icons"), Key("image-diffs"), Key("stage-diff-context-lines"),
		Key("input-modal-bottom"), Key("name-aliases"),
		Key("log"), Key("log", "important-refs"), Key("log", "hide-refs"),
		Key("agents", "leader-key"), Key("agents", "focus-sidebar-key"),
	} {
		out = append(out, Exclusion{Key: k, Reason: reasonTUIOnly})
	}
	return out
}
