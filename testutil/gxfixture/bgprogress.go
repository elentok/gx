package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/gxfixture/bgtx"
)

// bgSessionAge puts the session's creation before every seeded task: a task
// that starts before the run began reads as one of an earlier run.
const bgSessionAge = time.Hour

// seedBgProgress writes the session's seeded task files and transcript lines,
// then waits until the chat read lists the expected tasks.
func (f sessionSeeder) seedBgProgress(id string, s sessionSpec) error {
	path, err := f.chatFilePath(id, s.Harness)
	if err != nil {
		return err
	}
	now := time.Now()
	convID := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	seed := s.BgSeed(now, bgtx.Paths{
		Tasks:     filepath.Join(f.root, "tmp", "claude", filepath.Base(filepath.Dir(path)), convID, "tasks"),
		Subagents: strings.TrimSuffix(path, ".jsonl") + "/subagents",
		Rollouts:  filepath.Join(f.root, "home", ".codex", "sessions"),
	})
	if err := f.backdateSession(id, now.Add(-cmp.Or(seed.Age, bgSessionAge))); err != nil {
		return err
	}
	for _, file := range seed.Files {
		if err := f.writeBgFile(file); err != nil {
			return err
		}
	}
	if err := appendLines(path, seed.Lines); err != nil {
		return err
	}
	if seed.Version != "" {
		if err := setRolloutVersion(path, seed.Version); err != nil {
			return err
		}
	}
	return f.waitFor(id, fmt.Sprintf("%d tasks", s.BgTasks), func(_ wire.SessionInfo, c wire.SessionChatResponse) bool {
		return len(c.Tasks) == s.BgTasks && (!seed.Working || c.Current != nil)
	})
}

// setRolloutVersion rewrites the cli_version of the rollout's first line through
// a temp file and a rename, so a reader never sees a half-written file.
func setRolloutVersion(path, version string) error {
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

func (f sessionSeeder) writeBgFile(file bgtx.File) error {
	if err := os.MkdirAll(filepath.Dir(file.Path), 0o755); err != nil {
		return err
	}
	if file.Link != "" {
		if err := os.Symlink(file.Link, file.Path); err != nil {
			return err
		}
	} else {
		if err := os.WriteFile(file.Path, []byte(file.Content), 0o644); err != nil {
			return err
		}
		if !file.Mtime.IsZero() {
			if err := os.Chtimes(file.Path, file.Mtime, file.Mtime); err != nil {
				return err
			}
		}
	}
	if file.Key != "" {
		if f.taskFiles == nil {
			f.taskFiles = map[string]string{}
		}
		f.taskFiles[file.Key] = file.Path
	}
	return nil
}

// backdateSession moves the session's creation time back, and with it its
// run start.
func (f sessionSeeder) backdateSession(id string, at time.Time) error {
	dir := filepath.Join(f.root, "data", "gx", "sessions", id)
	meta, err := sessions.ReadMeta(dir)
	if err != nil {
		return err
	}
	meta.CreatedAt, meta.RestoredAt = at, time.Time{}
	return sessions.WriteMeta(dir, meta)
}
