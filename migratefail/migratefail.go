// Package migratefail keeps the record of failed ticket migrations: one entry
// per project, in a file in the state directory so it survives a server
// restart. cmd/ writes and clears it; server/ reads it.
package migratefail

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// FileName is the record's name inside the state directory.
const FileName = "migration-failures.json"

// Failure is one project's failed migration.
type Failure struct {
	Project string `json:"project"`
	Error   string `json:"error"`
}

// RestartLogName is the file `gx server restart --continue` writes its output
// to, next to the server's own stderr file.
const RestartLogName = "restart.log"

// RestartLogPath is the continue-mode restart log under stateDir.
func RestartLogPath(stateDir string) string { return filepath.Join(stateDir, RestartLogName) }

// Path is the record's file under stateDir.
func Path(stateDir string) string { return filepath.Join(stateDir, FileName) }

// Read returns the recorded failures sorted by project, keeping only projects
// in configured. A missing file is no failures.
func Read(stateDir string, configured []string) ([]Failure, error) {
	all, err := load(stateDir)
	if err != nil {
		return nil, err
	}
	var out []Failure
	for project, msg := range all {
		if slices.Contains(configured, project) {
			out = append(out, Failure{Project: project, Error: msg})
		}
	}
	slices.SortFunc(out, func(a, b Failure) int {
		switch {
		case a.Project < b.Project:
			return -1
		case a.Project > b.Project:
			return 1
		}
		return 0
	})
	return out, nil
}

// Record sets project's entry, replacing any earlier one.
func Record(stateDir, project, msg string) error {
	all, err := load(stateDir)
	if err != nil {
		return err
	}
	all[project] = msg
	return save(stateDir, all)
}

// Clear removes project's entry. It writes nothing when there is none.
func Clear(stateDir, project string) error {
	all, err := load(stateDir)
	if err != nil {
		return err
	}
	if _, ok := all[project]; !ok {
		return nil
	}
	delete(all, project)
	return save(stateDir, all)
}

func load(stateDir string) (map[string]string, error) {
	all := map[string]string{}
	data, err := os.ReadFile(Path(stateDir))
	if errors.Is(err, fs.ErrNotExist) {
		return all, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	return all, nil
}

// save writes through a rename so a watcher never reads half a file.
func save(stateDir string, all map[string]string) error {
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(stateDir, FileName+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), Path(stateDir))
}
