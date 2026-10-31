package harness

// SkillCommand is the text that types a skill into a session of the named
// harness: "/" for Claude, "$" for Codex. Any other name (the test fakes)
// types like Claude. It is the only place that builds a skill command; a test
// fails on a hardcoded skill literal elsewhere. arg is "" for none.
func SkillCommand(harnessName, skill, arg string) string {
	prefix := "/"
	if harnessName == string(Codex) {
		prefix = "$"
	}
	if arg == "" {
		return prefix + skill
	}
	return prefix + skill + " " + arg
}
