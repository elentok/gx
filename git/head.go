package git

import "github.com/elentok/gx/git/internal/run"

// ShortHead returns the abbreviated commit HEAD points at in dir.
func ShortHead(dir string) (string, error) {
	out, _, err := run.Exec(dir, []string{"rev-parse", "--short", "HEAD"})
	return out, err
}

// Toplevel returns the root of the checkout that contains dir.
func Toplevel(dir string) (string, error) {
	out, _, err := run.Exec(dir, []string{"rev-parse", "--show-toplevel"})
	return out, err
}
