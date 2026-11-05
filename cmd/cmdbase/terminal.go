package cmdbase

import (
	"io"
	"os"

	"github.com/mattn/go-isatty"
)

// IsTerminalWriter reports whether w is a terminal file.
func IsTerminalWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && IsTerminalFile(f)
}

// IsTerminalFile reports whether f is a terminal (or a Cygwin pty).
func IsTerminalFile(f *os.File) bool {
	fd := f.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
