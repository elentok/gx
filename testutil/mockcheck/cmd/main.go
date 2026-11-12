// Command mockcheck validates a mock check's result file; see mockcheck.Validate.
//
//	go run ./testutil/mockcheck/cmd <results-file>
package main

import (
	"fmt"
	"os"

	"github.com/elentok/gx/testutil/mockcheck"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: mockcheck <results-file>")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	problems := mockcheck.Validate(string(data))
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, p)
	}
	if len(problems) > 0 {
		os.Exit(1)
	}
}
