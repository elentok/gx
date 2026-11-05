package nativerunner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/harness"
)

type adoption int

const (
	adoptGone   adoption = iota // holder pid is not alive
	adoptSilent                 // holder pid is alive but its socket doesn't answer
	adoptedLive                 // session is tracked and drivable
)

// adopt tracks the holder under label's dir. It is the one adopt path for
// Reattach, Find and List.
func (p *Pty) adopt(label string) (agentrunner.Session, adoption, error) {
	if s, ok := p.reg.tracked(label); ok {
		return s, adoptedLive, nil
	}
	dir := filepath.Join(p.Root, label)
	meta, err := readMeta(dir)
	if err != nil {
		return agentrunner.Session{}, 0, err
	}
	live, err := ptyAlive(orPS(p.Procs), meta)
	if err != nil {
		return agentrunner.Session{}, 0, err
	}
	if !live {
		return agentrunner.Session{}, adoptGone, nil
	}
	// A holder whose socket stays silent can't be driven.
	holder, err := DialHolder(dir)
	if err != nil {
		return p.reg.trackSilent(p.newSession(label, meta)), adoptSilent, nil
	}
	ss := p.newSession(label, meta)
	ss.holder = holder
	// The probe's memory of the launch override died with the old server.
	if meta.EditorModeNormal {
		harness.NoteEditorOverride(meta.SessionID, nil)
	}
	// A concurrent adopt of label may have won meanwhile.
	s, _ := p.reg.putUnlessLive(label, ss, func() { go p.launcher().watchExit(ss, meta) })
	return s, adoptedLive, nil
}

func (p *Pty) newSession(label string, meta Meta) *ptySession {
	return &ptySession{
		session:     agentrunner.Session{Label: label, ID: label, SessionID: meta.SessionID},
		epic:        meta.Epic,
		kind:        meta.Harness,
		holderPID:   meta.HolderPID,
		claudePID:   meta.PID,
		holderStart: meta.HolderPIDStart,
		claudeStart: meta.PIDStart,
		exited:      make(chan struct{}),
	}
}

// heldLabel decides what a live holder under opts.Label means for Start: the
// holder's session when it is adopted, a LabelTakenError when it blocks the
// launch, and neither when the dir has no live holder. It never touches the
// holder's files.
func (p *Pty) heldLabel(opts agentrunner.StartOptions) (agentrunner.Session, bool, error) {
	s, outcome, err := p.adopt(opts.Label)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return agentrunner.Session{}, false, nil
	}
	if err != nil {
		return agentrunner.Session{}, false, err
	}
	if outcome == adoptGone {
		return agentrunner.Session{}, false, nil
	}
	meta, err := readMeta(filepath.Join(p.Root, opts.Label))
	if err != nil {
		return agentrunner.Session{}, false, err
	}
	if outcome == adoptedLive && !opts.NoAdopt && filepath.Clean(meta.Cwd) == filepath.Clean(opts.Cwd) {
		return s, true, nil
	}
	return agentrunner.Session{}, false, &agentrunner.LabelTakenError{
		HolderCwd: meta.Cwd,
		Err:       fmt.Errorf("%s: holder pid %d runs in %s", opts.Label, meta.HolderPID, meta.Cwd),
	}
}

// Find returns label's agent, adopting a live holder an earlier server left
// behind. A holder with a live pid and a silent socket is returned dead.
func (p *Pty) Find(label string) (agentrunner.Session, bool, error) {
	if ss, ok := p.reg.get(label); ok {
		return ss.session, true, nil
	}
	s, outcome, err := p.adopt(label)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return agentrunner.Session{}, false, nil
	}
	if err != nil {
		return agentrunner.Session{}, false, err
	}
	if outcome == adoptGone {
		return agentrunner.Session{}, false, nil
	}
	return s, true, nil
}

// List returns epic's agents, adopting live holders on disk that this value
// doesn't track yet.
func (p *Pty) List(epic string) ([]agentrunner.Session, error) {
	labels := p.reg.labelsOf(epic)
	entries, err := os.ReadDir(p.Root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		meta, err := readMeta(filepath.Join(p.Root, e.Name()))
		if err == nil && meta.Epic == epic {
			labels[e.Name()] = struct{}{}
		}
	}
	var out []agentrunner.Session
	for label := range labels {
		s, ok, err := p.Find(label)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b agentrunner.Session) int { return strings.Compare(a.Label, b.Label) })
	return out, nil
}
