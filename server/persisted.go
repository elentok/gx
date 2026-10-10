package server

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// PersistedActive lists the ticket addresses the state dir's queue and run
// files hold, for a caller that cannot reach a running server. A missing file
// is an empty list; an unreadable one is an error, so the caller never mistakes
// "could not tell" for "not active".
func PersistedActive(stateDir string) ([]string, error) {
	var out []string
	for _, name := range []string{queueFileName, runsFileName} {
		data, err := os.ReadFile(filepath.Join(stateDir, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var entries []struct {
			Address string `json:"address"`
		}
		if err := json.Unmarshal(data, &entries); err != nil {
			return nil, err
		}
		for _, e := range entries {
			out = append(out, e.Address)
		}
	}
	return out, nil
}
