package chat

import (
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/transcript"
)

// codexChild is what a Codex subagent's own rollout shows.
type codexChild struct {
	Model string
	// Tokens is the last cumulative total, never a sum of token_count lines.
	Tokens int
}

func readCodexChild(path string) (codexChild, error) {
	var out codexChild
	turn, ok, err := codexsession.ReadTurnModel(path)
	if err != nil {
		return codexChild{}, err
	}
	if ok {
		out.Model = turn.Model
	}
	usage, err := codexsession.ReadContextUsage(path)
	if err != nil {
		return codexChild{}, err
	}
	out.Tokens = int(usage.Total.Total)
	return out, nil
}

// codexChildPath finds a child's rollout by its thread id, once per id.
func (c *Chat) codexChildPath(threadID string, meta Meta) (string, bool) {
	return c.transcripts.Resolve(threadID, meta.Status.State, time.Now(), c.cfg.TranscriptMissTTL, func() (string, bool) {
		home, err := c.src.CodexHome()
		if err != nil {
			c.log.Warn("codex home", "session", meta.ID, "err", err)
			return "", false
		}
		lookup := codexsession.RolloutPathIn
		if c.rolloutPath != nil {
			lookup = c.rolloutPath
		}
		path, ok, err := lookup(home, threadID)
		if err != nil {
			c.log.Warn("find child rollout", "session", meta.ID, "thread", threadID, "err", err)
			return "", false
		}
		return path, ok
	})
}

// withCodexChildren returns records with each Codex subagent's model and token
// total filled from its child rollout. Like withSubagentDetails it runs on every
// chat read and not in the snapshot: the child grows while the parent is quiet.
// A missing child leaves the started event's model and no tokens. It copies what
// it changes: the records belong to the cached snapshot.
func (c *Chat) withCodexChildren(records []transcript.TaskRecord, meta Meta) []transcript.TaskRecord {
	compute := readCodexChild
	if c.readCodexChild != nil {
		compute = c.readCodexChild
	}
	var out []transcript.TaskRecord
	for i, r := range records {
		if r.Kind != transcript.TaskKindSubagent || r.ID == "" {
			continue
		}
		path, ok := c.codexChildPath(r.ID, meta)
		if !ok {
			continue
		}
		d, err := c.codexChildren.Read(path, compute)
		if err != nil || d == (codexChild{}) {
			continue
		}
		if out == nil {
			out = make([]transcript.TaskRecord, len(records))
			copy(out, records)
		}
		r.Tokens = d.Tokens
		if d.Model != "" {
			r.Model = d.Model
		}
		out[i] = r
	}
	if out == nil {
		return records
	}
	return out
}
