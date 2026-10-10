package cmd

import (
	"fmt"
	"io"

	"github.com/elentok/gx/version"
)

func getVersion() string {
	return version.Get()
}

func runVersion(w io.Writer) error {
	fmt.Fprintf(w, "gx %s\n", getVersion())
	return nil
}
