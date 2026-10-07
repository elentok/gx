// Command agentfakeworker is testutil/agentfake's RunWorker, built as the
// literal binary name "claude" so a server-launched `herdr agent start --kind
// claude` execs it. It ignores its args: the server passes real-agent flags
// (--permission-mode ...) that the fake has no use for.
package main

import (
	"fmt"
	"os"

	"github.com/elentok/gx/testutil/agentfake"
)

func main() {
	if err := agentfake.RunWorker(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
