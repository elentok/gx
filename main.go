package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/elentok/gx/cmd"
)

func main() {
	if err := cmd.Execute(os.Args[1:]); err != nil {
		var exitErr *cmd.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
