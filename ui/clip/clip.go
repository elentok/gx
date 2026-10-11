// Package clip writes to the system clipboard.
package clip

import (
	"testing"

	"github.com/atotto/clipboard"
)

// Write copies s to the system clipboard. Inside a test binary it does
// nothing, so tests that trigger a yank without stubbing their package's
// clipboard seam never clobber the developer's real clipboard.
func Write(s string) error {
	if testing.Testing() {
		return nil
	}
	return clipboard.WriteAll(s)
}
