package codexsession

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
)

// ChildMeta is what a Codex subagent's rollout says about itself in its
// session_meta line.
type ChildMeta struct {
	// AgentRole is the agent_type the spawn call passed; empty when it passed none.
	AgentRole string
	// HistoryStartOrdinal is where the child's own lines start: the lines
	// before it are a copy of the parent's history.
	HistoryStartOrdinal int
}

// ReadChildMeta reads the session_meta line of the rollout at path, which is
// its first line.
func ReadChildMeta(path string) (ChildMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return ChildMeta{}, err
	}
	defer f.Close()
	raw, err := bufio.NewReader(f).ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return ChildMeta{}, err
	}
	var line struct {
		Type    string `json:"type"`
		Payload struct {
			StartOrdinal int `json:"subagent_history_start_ordinal"`
			Source       struct {
				Subagent struct {
					ThreadSpawn struct {
						AgentRole *string `json:"agent_role"`
					} `json:"thread_spawn"`
				} `json:"subagent"`
			} `json:"source"`
		} `json:"payload"`
	}
	if json.Unmarshal(raw, &line) != nil || line.Type != "session_meta" {
		return ChildMeta{}, nil
	}
	m := ChildMeta{HistoryStartOrdinal: line.Payload.StartOrdinal}
	if role := line.Payload.Source.Subagent.ThreadSpawn.AgentRole; role != nil {
		m.AgentRole = *role
	}
	return m, nil
}
