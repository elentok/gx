// Command agentfake is the entry point for testutil/agentfake, built as the
// literal binary name "claude" by e2e tests (see e2e's agentfakeBinary
// helper) so `herdr agent start --kind claude` execs it instead of a real
// Claude Code.
package main

import (
	"fmt"
	"os"

	"github.com/elentok/gx/testutil/agentfake"
)

func main() {
	opts, err := agentfake.ParseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := agentfake.Run(opts, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
