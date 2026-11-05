package holder

import (
	"os"
	"path/filepath"
	"strings"
)

// HolderErrFile holds the holder's stderr. It stays after a failed start, so
// the launch error and a person can read why the holder never listened.
const HolderErrFile = "holder.err"

// CreateHolderErr opens a fresh holder.err in dir for the holder's stderr.
func CreateHolderErr(dir string) (*os.File, error) {
	return os.Create(filepath.Join(dir, HolderErrFile))
}

// HolderErrText is what the holder wrote to stderr, empty when nothing.
func HolderErrText(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, HolderErrFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
