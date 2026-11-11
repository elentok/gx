// Command agentfaketui is testutil/agentfake's fake interactive claude, built
// as the literal binary name "claude" by e2e tests so a real gx server's
// claude session runs it.
package main

import "github.com/elentok/gx/testutil/agentfake/tuiclaude"

func main() {
	tuiclaude.TUIClaude()
}
