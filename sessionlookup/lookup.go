// Package sessionlookup resolves an agent session id to markdown, so every
// reader (evolve digests, the TUI) finds a session's log the same way.
package sessionlookup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/claudehistory"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/transcript"
)

// Where a session's markdown came from, in lookup order. Missing means no log
// was found, so the gap shows instead of passing for a clean run.
const (
	Claude  = "claude"
	Native  = "native"
	Codex   = "codex"
	Missing = "missing"
)

// Roots are the directories searched. An empty root skips its source.
type Roots struct {
	Home      string
	CodexHome string
}

// DefaultRoots are the user's own roots: $HOME, and $CODEX_HOME falling back
// to ~/.codex. An unknown home leaves its roots empty.
func DefaultRoots() Roots {
	var r Roots
	if home, err := os.UserHomeDir(); err == nil {
		r.Home = home
		r.CodexHome = filepath.Join(home, ".codex")
	}
	if v := os.Getenv("CODEX_HOME"); v != "" {
		r.CodexHome = v
	}
	return r
}

// Session is one resolved session.
type Session struct {
	Source    string
	Markdown  string
	Subagents []Subagent
}

// Subagent is one subagent transcript of a claude session.
type Subagent struct{ Name, Markdown string }

// Lookup tries the claude transcript, then the native agent directory at
// agentDir ("" skips it), then the codex rollout.
func Lookup(r Roots, agentDir, id string) (Session, error) {
	if r.Home != "" {
		matches, err := transcript.FindByID(r.Home, id)
		if err != nil {
			return Session{}, err
		}
		if len(matches) > 0 {
			return claudeSession(matches[0])
		}
	}
	if agentDir != "" {
		d, err := nativerunner.OpenDir(agentDir)
		switch {
		// A dir that reads the transcript has nothing more to offer: the
		// transcript search above already missed, and its own log is raw
		// terminal bytes.
		case err == nil && d.Meta.SessionID == id && !d.ReadsTranscript() && !d.Pruned():
			md, err := claudehistory.ExportMarkdown(d.Log())
			if err != nil {
				return Session{}, err
			}
			return Session{Source: Native, Markdown: md}, nil
		case err != nil && !errors.Is(err, agentrunner.ErrNotFound):
			return Session{}, err
		}
	}
	if r.CodexHome != "" {
		md, ok, err := codexsession.ExportMarkdownIn(r.CodexHome, id)
		if err != nil {
			return Session{}, err
		}
		if ok {
			return Session{Source: Codex, Markdown: md}, nil
		}
	}
	return Session{Source: Missing}, nil
}

// claudeSession exports the transcript at path and the ones in
// "<session>/subagents" next to it.
func claudeSession(path string) (Session, error) {
	md, err := claudehistory.ExportMarkdown(path)
	if err != nil {
		return Session{}, err
	}
	files, err := filepath.Glob(filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents", "*.jsonl"))
	if err != nil {
		return Session{}, err
	}
	s := Session{Source: Claude, Markdown: md}
	for _, f := range files {
		md, err := claudehistory.ExportMarkdown(f)
		if err != nil {
			return Session{}, err
		}
		s.Subagents = append(s.Subagents, Subagent{Name: strings.TrimSuffix(filepath.Base(f), ".jsonl"), Markdown: md})
	}
	return s, nil
}
