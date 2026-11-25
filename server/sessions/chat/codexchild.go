package chat

import (
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/transcript"
)

// codexChild is what a Codex subagent's own rollout shows.
type codexChild struct {
	Model string
	// Tokens is the last cumulative total, never a sum of token_count lines.
	Tokens int
	// Role is the child's agent_role; empty for a null role.
	Role string
}

func readCodexChild(path string) (codexChild, error) {
	meta, err := codexsession.ReadChildMeta(path)
	if err != nil {
		return codexChild{}, err
	}
	out := codexChild{Role: meta.AgentRole}
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
	return c.transcripts.Resolve(threadID, meta.Status.State, c.clock(), c.cfg.TranscriptMissTTL, func() (string, bool) {
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
	if c.reads.codexChild != nil {
		compute = c.reads.codexChild
	}
	return enrichTasks(records, enrichSpec[codexChild]{
		cache: &c.codexChildren, compute: compute,
		path: func(r transcript.TaskRecord) (string, bool) {
			if r.Kind != transcript.TaskKindSubagent {
				return "", false
			}
			ref, ok := c.codexSubagent(r, meta)
			return ref.Path, ok
		},
		usable: func(d codexChild) bool { return d != (codexChild{}) },
		merge: func(r *transcript.TaskRecord, d codexChild) {
			r.Tokens, r.SubagentType = d.Tokens, d.Role
			if d.Model != "" {
				r.Model = d.Model
			}
		},
	})
}
