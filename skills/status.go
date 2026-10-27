package skills

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// DevSourcePrefix starts a dev install's manifest Source; the checkout root
// follows it.
const DevSourcePrefix = "gx dev checkout: "

// DevRoot returns the checkout a dev install links from, and false when m is
// not a dev install.
func DevRoot(m Manifest) (string, bool) {
	if m.Mode != ModeSymlink {
		return "", false
	}
	root, ok := strings.CutPrefix(m.Source, DevSourcePrefix)
	return root, ok && root != ""
}

// StatusPath is the file `gx server start` records its skill install's
// problem in, so the server can show it.
func StatusPath(stateDir string) string {
	return filepath.Join(stateDir, "skills-install.json")
}

type problemRecord struct {
	Problem string `json:"problem"`
}

// WriteProblem records problem at path, or removes the file when problem is
// empty.
func WriteProblem(path, problem string) error {
	if problem == "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, err := json.Marshal(problemRecord{Problem: problem})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// ReadProblem returns the problem recorded at path, or "" when there is none.
func ReadProblem(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var st problemRecord
	if json.Unmarshal(b, &st) != nil {
		return ""
	}
	return st.Problem
}
