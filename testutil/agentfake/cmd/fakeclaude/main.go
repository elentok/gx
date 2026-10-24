// Command fakeclaude is the one fake claude binary e2e fixtures install as
// "claude" on PATH. Today it runs the interactive TUI fake; worker and
// design-chat modes join it here.
package main

import "github.com/elentok/gx/testutil/agentfake"

func main() {
	agentfake.TUIClaude()
}
