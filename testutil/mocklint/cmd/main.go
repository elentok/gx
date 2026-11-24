// Command mocklint checks stored mock files; see mocklint.Lint.
//
//	go run ./testutil/mocklint/cmd <file>...
package main

import (
	"fmt"
	"os"

	"github.com/elentok/gx/testutil/mocklint"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: mocklint <file>...")
		os.Exit(2)
	}
	bad := false
	for _, path := range os.Args[1:] {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		for _, f := range mocklint.Lint(path, string(data)) {
			fmt.Fprintln(os.Stderr, f)
			bad = true
		}
	}
	if bad {
		os.Exit(1)
	}
}
