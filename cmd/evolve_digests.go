package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/claudehistory"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/transcript"
)

// Where a session's digest came from, in lookup order. digestMissing means no
// log was found, so the gap shows instead of passing for a clean run.
const (
	digestClaude  = "claude"
	digestNative  = "native"
	digestCodex   = "codex"
	digestMissing = "missing"
)

// Digest kinds.
const (
	digestMain     = "main"
	digestConflict = "conflict"
	digestSub      = "subagent"
)

// InputsDigest is one session's digest file. File is empty when missing.
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

// digestSession digests one session (and its subagents) by trying the claude
// transcript, then the native agent log, then the codex rollout.
func digestSession(env evolveEnv, t evolveTarget, dir, ticket, session, kind string) ([]InputsDigest, error) {
	md, source, subs, err := lookupSession(env, t, ticket, session)
	if err != nil {
		return nil, err
	}
	if source == digestMissing {
		return []InputsDigest{{Session: session, Kind: kind, Source: digestMissing}}, nil
	}
	base := fmt.Sprintf("%s-%s", ticket, session)
	write := func(name, content string) (string, error) {
		return name, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
	}
	file, err := write(base+".md", md)
	if err != nil {
		return nil, err
	}
	out := []InputsDigest{{Session: session, Kind: kind, Source: source, File: file}}
	for _, sub := range subs {
		file, err := write(fmt.Sprintf("%s-sub-%s.md", base, sub.name), sub.md)
		if err != nil {
			return nil, err
		}
		out = append(out, InputsDigest{Session: session, Kind: digestSub, Source: source, File: file})
	}
	return out, nil
}

type subDigest struct{ name, md string }

func lookupSession(env evolveEnv, t evolveTarget, ticket, session string) (md, source string, subs []subDigest, err error) {
	if env.homeDir != "" {
		matches, err := transcript.FindByID(env.homeDir, session)
		if err != nil {
			return "", "", nil, err
		}
		if len(matches) > 0 {
			if md, err = claudehistory.ExportMarkdown(matches[0]); err != nil {
				return "", "", nil, err
			}
			subs, err = subagentDigests(matches[0])
			return md, digestClaude, subs, err
		}
	}
	if env.stateDir != "" {
		label, _, _ := ralphloop.IterationIdentity(t.name, ticket, "")
		h := &nativerunner.Headless{Root: nativerunner.AgentsRoot(env.stateDir, t.project)}
		meta, _, err := h.Inspect(label)
		switch {
		case err == nil && meta.SessionID == session && !nativerunner.Pruned(filepath.Join(h.Root, label)):
			if md, err = claudehistory.ExportMarkdown(filepath.Join(h.Root, label, nativerunner.OutFile)); err != nil {
				return "", "", nil, err
			}
			return md, digestNative, nil, nil
		case err != nil && !errors.Is(err, agentrunner.ErrNotFound):
			return "", "", nil, err
		}
	}
	if env.codexHome != "" {
		md, ok, err := codexsession.ExportMarkdownIn(env.codexHome, session)
		if err != nil {
			return "", "", nil, err
		}
		if ok {
			return md, digestCodex, nil, nil
		}
	}
	return "", digestMissing, nil, nil
}

// subagentDigests exports the transcripts in "<session>/subagents" next to the
// main transcript at path.
func subagentDigests(path string) ([]subDigest, error) {
	files, err := filepath.Glob(filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	var subs []subDigest
	for _, f := range files {
		md, err := claudehistory.ExportMarkdown(f)
		if err != nil {
			return nil, err
		}
		subs = append(subs, subDigest{name: strings.TrimSuffix(filepath.Base(f), ".jsonl"), md: md})
	}
	return subs, nil
}
