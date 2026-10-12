package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/sessionlookup"
)

// Digest kinds.
const (
	digestMain     = "main"
	digestConflict = "conflict"
	digestSub      = "subagent"
)

// InputsDigest is one session's digest file. Source is a sessionlookup source;
// File is empty when missing.
type InputsDigest struct {
	Session string `json:"session"`
	Kind    string `json:"kind"`
	Source  string `json:"source"`
	File    string `json:"file,omitempty"`
}

// writeDigests writes a text-only digest per session of every row into dir and
// records them on the rows.
func writeDigests(env evolveEnv, t evolveTarget, dir string, table *InputsTable) error {
	for i := range table.Rows {
		row := &table.Rows[i]
		sessions := []struct{ id, kind string }{}
		for _, id := range row.Sessions {
			sessions = append(sessions, struct{ id, kind string }{id, digestMain})
		}
		for _, id := range row.ConflictSessions {
			sessions = append(sessions, struct{ id, kind string }{id, digestConflict})
		}
		for _, s := range sessions {
			ds, err := digestSession(env, t, dir, row.ID, s.id, s.kind)
			if err != nil {
				return fmt.Errorf("ticket %s session %s: %w", row.ID, s.id, err)
			}
			row.Digests = append(row.Digests, ds...)
		}
	}
	return nil
}

// digestSession digests one session and its subagents.
func digestSession(env evolveEnv, t evolveTarget, dir, ticket, session, kind string) ([]InputsDigest, error) {
	agentDir := ""
	if env.stateDir != "" {
		label, _, _ := ralphloop.IterationIdentity(t.name, ticket, "")
		agentDir = filepath.Join(nativerunner.AgentsRoot(env.stateDir, t.project), label)
	}
	s, err := sessionlookup.Lookup(sessionlookup.Roots{Home: env.homeDir, CodexHome: env.codexHome}, agentDir, session)
	if err != nil {
		return nil, err
	}
	if s.Source == sessionlookup.Missing {
		return []InputsDigest{{Session: session, Kind: kind, Source: s.Source}}, nil
	}
	base := fmt.Sprintf("%s-%s", ticket, session)
	write := func(name, content string) (string, error) {
		return name, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
	}
	file, err := write(base+".md", s.Markdown)
	if err != nil {
		return nil, err
	}
	out := []InputsDigest{{Session: session, Kind: kind, Source: s.Source, File: file}}
	for _, sub := range s.Subagents {
		file, err := write(fmt.Sprintf("%s-sub-%s.md", base, sub.Name), sub.Markdown)
		if err != nil {
			return nil, err
		}
		out = append(out, InputsDigest{Session: session, Kind: digestSub, Source: s.Source, File: file})
	}
	return out, nil
}
