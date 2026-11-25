package main

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/gxfixture/bgtx"
	"github.com/elentok/gx/testutil/gxfixture/output"
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
		if err := file.Write(); err != nil {
			return err
		}
		if file.Key != "" {
			if f.taskFiles == nil {
				f.taskFiles = map[string]string{}
			}
			f.taskFiles[file.Key] = file.Path
		}
	}
	for _, file := range seed.Reserved {
		if err := os.MkdirAll(filepath.Dir(file.Path), 0o755); err != nil {
			return err
		}
		if f.taskFiles == nil {
			f.taskFiles = map[string]string{}
		}
		f.taskFiles[file.Key] = file.Path
	}
	if err := appendLines(path, seed.Lines); err != nil {
		return err
	}
	if seed.Version != "" {
		if err := bgtx.SetRolloutVersion(path, seed.Version); err != nil {
			return err
		}
	}
	if seed.Ready != nil {
		f.subagents = &output.SubagentsOutput{SessionID: id, Calls: seed.Calls}
		if err := f.waitFor(id, "the subagent calls", func(_ wire.SessionInfo, c wire.SessionChatResponse) bool { return seed.CallsShown(c) }); err != nil {
			return err
		}
		// recordTranscript leaves the file of a seeded session: the lines above need it.
		if s.NoTranscript {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
		return nil
	}
	return f.waitFor(id, fmt.Sprintf("%d tasks", s.BgTasks), func(_ wire.SessionInfo, c wire.SessionChatResponse) bool {
		return len(c.Tasks) == s.BgTasks && (!seed.Working || c.Current != nil)
	})
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
