// Package codexsetup installs the Codex approvals gx relies on: a managed
// rules file for git commands and auto-approval of the gopls MCP tools.
package codexsetup

import "path/filepath"

const (
	// Marker is the first line of the managed rules file.
	Marker = "# Managed by gx codex setup"
	// OldMarker is what scripts/configure-codex-global-approvals.sh wrote;
	// files carrying it are managed too, so an old install updates in place.
	OldMarker = "# Managed by configure-codex-global-approvals.sh"
	// RulesFile is the managed rules file name under $CODEX_HOME/rules.
	RulesFile = "allow-git-commands.rules"
)

// RulesPath is the managed rules file under codexHome.
func RulesPath(codexHome string) string {
	return filepath.Join(codexHome, "rules", RulesFile)
}

const rulesBody = `# Codex evaluates each literal argument after these prefixes separately.
prefix_rule(
    pattern = ["git", ["add", "commit", "diff", "status", "log"]],
    decision = "allow",
    justification = "User-approved Git staging, commits, and repository inspection",
    match = [
        "git add .",
        "git add path/to/file",
        "git commit -m example",
        "git diff --cached",
        "git status --short",
        "git log --oneline -5",
    ],
    not_match = [
        "git push",
        "git reset --hard",
    ],
)

prefix_rule(
    pattern = ["git", "cherry-pick"],
    decision = "allow",
    justification = "User-approved Git cherry-pick operations",
    match = [
        "git cherry-pick a1b2c3d",
        "git cherry-pick --continue",
        "git cherry-pick --abort",
    ],
    not_match = ["git merge feature"],
)

# An environment assignment makes the command opaque to Codex command splitting,
# so allow only this exact shell payload across common POSIX shell paths.
prefix_rule(
    pattern = [
        ["bash", "/bin/bash", "/usr/bin/bash", "zsh", "/bin/zsh", "/usr/bin/zsh", "sh", "/bin/sh", "/usr/bin/sh"],
        ["-lc", "-c"],
        "GIT_EDITOR=true git cherry-pick --continue",
    ],
    decision = "allow",
    justification = "User-approved non-interactive continuation of an in-progress Git cherry-pick",
)
`

// RulesContent is the full managed rules file.
func RulesContent() string { return Marker + "\n" + rulesBody }
