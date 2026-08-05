package skills

import "path/filepath"

// ClaudeSkillsRoot is Claude Code's user skill discovery root under home.
func ClaudeSkillsRoot(home string) string {
	return filepath.Join(home, ".claude", "skills")
}

// CodexSkillsRoot is Codex's user custom-prompt discovery root under home.
// Bundle's files keep the same relative layout under both agent roots (see
// gx.md's "Layout" section), so a skill's relative references - e.g.
// "../gx-local-tracker.md" - resolve identically under Claude and Codex.
func CodexSkillsRoot(home string) string {
	return filepath.Join(home, ".codex", "prompts")
}
