// Command gh is the fake gh that PR-flow tests put first on PATH.
package main

import (
	"os"

	"github.com/elentok/gx/testutil/ghfake"
)

func main() { os.Exit(ghfake.Run(os.Args[1:], os.Stdout, os.Stderr)) }
