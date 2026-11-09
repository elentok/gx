package registry

const (
	reasonTUIOnly = "TUI only, not shown on the web page"
	reasonIgnored = "accepted in project.json, ignored"
)

// Exclusions lists config keys deliberately left out of the registry.
func Exclusions() []Exclusion {
	out := []Exclusion{
		{Key: Key("$schema"), Reason: "not a setting"},
		{Key: Key("agents", "resume-on-start"), Reason: "legacy alias"},
		{Key: Key("notifications"), Project: true,
			Reason: "accepted in project.json, deliberately not editable on the web (secrets); ProjectFile still loads it and the page leaves an existing block untouched"},
	}
	for _, k := range []string{"restore-sessions-on-start", "banners", "sounds", "claude-args", "fix-agent-remote-control"} {
		out = append(out, Exclusion{Key: Key("agents", k), Project: true, Reason: reasonIgnored})
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
