package bgtx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// SetRolloutVersion rewrites the cli_version of the rollout's first line through
// a temp file and a rename, so a reader never sees a half-written file.
func SetRolloutVersion(path, version string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	first, rest, _ := bytes.Cut(data, []byte("\n"))
	var line map[string]any
	if err := json.Unmarshal(first, &line); err != nil {
		return fmt.Errorf("rollout %s: first line: %w", path, err)
	}
	payload, ok := line["payload"].(map[string]any)
	if !ok {
		return fmt.Errorf("rollout %s: first line has no payload", path)
	}
	payload["cli_version"] = version
	head, err := json.Marshal(line)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, slices.Concat(head, []byte("\n"), rest), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Write puts the file on disk: a symlink when Link is set, else Content with
// its Mtime.
func (f File) Write() error {
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return err
	}
	if f.Link != "" {
		return os.Symlink(f.Link, f.Path)
	}
	if err := os.WriteFile(f.Path, []byte(f.Content), 0o644); err != nil {
		return err
	}
	if f.Mtime.IsZero() {
		return nil
	}
	return os.Chtimes(f.Path, f.Mtime, f.Mtime)
}
