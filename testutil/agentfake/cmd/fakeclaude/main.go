// Command fakeclaude is the one fake claude binary e2e fixtures install as
// "claude" on PATH. It runs the interactive TUI fake; in a worktree carrying
// agentfake.WorkerConfigFile it also does a ticket's work for each skill
// prompt. Design-chat mode joins it here.
package main

import "github.com/elentok/gx/testutil/agentfake"

func main() {
	agentfake.TUIClaude()
}
