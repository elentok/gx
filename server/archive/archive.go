// Package archive moves finished loop epics into the project's .archive
// folder: it stops their live Sessions, clears the hold, stores the workspace
// in the frontmatter and re-keys the Sessions to the archive key.
package archive

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/cleanupscan"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/workspaces"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketgit"
)

// DesignNotFinishedText is the refusal of a Design before its design step is done.
const DesignNotFinishedText = "a Design archives once its design step is done"

// Deps is what Archive reads from the Server. The embedded Directory is the
// one the workspace store uses, so Hold and Archive share one stop path.
type Deps struct {
	workspaces.Directory
	TicketStore string
	Events      *broker.Broker
	// Workspace names the workspace a key is in; Unhold clears its hold mark.
	Workspace func(key string) string
	Unhold    func(key string) error
	// SetMember puts a key in a workspace (an unknown one is an error); Remove
	// returns it to Default.
	SetMember func(key, workspace string) error
	Remove    func(key string) error
	// CleanupDesign removes a Design's worktree and branch when it allows it.
	CleanupDesign func(project, epic string)
	// RekeySession rewrites a Session's ticket and fix_of through rekey.
	RekeySession func(id string, rekey func(ticket, fixOf string) (string, string)) error
}

// Archive runs the archive writes.
type Archive struct {
	d  Deps
	mu sync.Mutex // one archive at a time, so two clashes cannot take the same suffix
}

// New builds Archive.
func New(d Deps) *Archive { return &Archive{d: d} }

// Epics archives each target on its own, in order. A refusal is a result and
// blocks no other target.
func (a *Archive) Epics(req wire.ArchiveRequest) (wire.TargetsResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	res := wire.TargetsResult{Results: make([]wire.TargetResult, 0, len(req.Targets))}
	for _, target := range req.Targets {
		r, err := a.one(target, req.Yes, req.DryRun)
		if err != nil {
			return wire.TargetsResult{}, err
		}
		res.Results = append(res.Results, r)
	}
	return res, nil
}

// Cleanup maps the one-epic alias onto the same code.
func (a *Archive) Cleanup(project, epic string, yes bool) (wire.CleanupWriteResult, error) {
	res, err := a.Epics(wire.ArchiveRequest{Targets: []string{project + ":" + epic}, Yes: yes})
	if err != nil || len(res.Results) == 0 {
		return wire.CleanupWriteResult{}, err
	}
	if r := res.Results[0]; r.Refused() {
		return wire.CleanupWriteResult{Refused: true, Reason: r.Reason, Message: r.Message}, nil
	}
	return wire.CleanupWriteResult{}, nil
}

func (a *Archive) one(target string, yes, dryRun bool) (wire.TargetResult, error) {
	refuse := func(reason, msg string) (wire.TargetResult, error) {
		return wire.RefusedTarget(target, reason, msg), nil
	}
	project, epic, ok := tickets.ParseEpicAddress(strings.TrimSpace(target))
	if !ok {
		return refuse(wire.ReasonCleanupUnknown, "not an epic address (want project:epic)")
	}
	dir, err := tickets.ProjectDirByName(a.d.TicketStore, project)
	if err != nil {
		if _, isAddr := errors.AsType[*tickets.AddressError](err); isAddr {
			return refuse(wire.ReasonCleanupUnknown, err.Error())
		}
		return wire.TargetResult{}, err
	}
	repoOf := func() (string, error) {
		repo := ticketgit.ProjectRepoRoot(a.d.TicketStore, tickets.ProjectName(dir))
		if repo == "" {
			return "", fmt.Errorf("project %q has no repo", project)
		}
		return repo, nil
	}
	scan := func(name string) (*cleanupscan.EpicScan, error) {
		repo, err := repoOf()
		if err != nil {
			return nil, err
		}
		return cleanupscan.ScanEpic(a.d.TicketStore, repo, name)
	}

	// A target given as the -impl address resolves to its Design's row.
	base, asImpl := epic, false
	if d, ok := strings.CutSuffix(epic, tickets.ImplEpicSuffix); ok && a.d.DesignExists(project+":"+d) {
		base, asImpl = d, true
	}
	halves := []half{{epic: base, key: project + ":" + base}}
	var onTrunk bool
	if a.d.DesignExists(halves[0].key) {
		step := tickets.DesignStep(tickets.LoadEpicTicketMD(filepath.Join(dir, base)).DesignStep)
		if step != tickets.StepDone {
			return refuse(wire.ReasonArchiveNotFinished, DesignNotFinishedText)
		}
		if a.d.RootQueued(halves[0].key) {
			return refuse(wire.ReasonCleanupQueued, "the epic is queued")
		}
		impl := base + tickets.ImplEpicSuffix
		if _, err := os.Stat(filepath.Join(dir, impl)); err == nil {
			implKey := project + ":" + impl
			if a.d.RootQueued(implKey) {
				return refuse(wire.ReasonArchiveImplNotArchivable, "the -impl epic is queued")
			}
			row, err := scan(impl)
			if err != nil {
				return wire.TargetResult{}, err
			}
			if row == nil || !row.AllDone {
				return refuse(wire.ReasonArchiveImplNotArchivable, "the -impl epic has tickets that are not done")
			}
			onTrunk = row.MergedToTrunk
			halves = append(halves, half{epic: impl, key: implKey})
		}
	} else {
		if a.d.RootQueued(halves[0].key) {
			return refuse(wire.ReasonCleanupQueued, "the epic is queued")
		}
		row, err := scan(base)
		if err != nil {
			return wire.TargetResult{}, err
		}
		switch {
		case row == nil:
			return refuse(wire.ReasonCleanupUnknown, "no such epic")
		case !row.AllDone:
			return refuse(wire.ReasonCleanupNotDone, "the epic has tickets that are not done")
		}
		onTrunk = row.MergedToTrunk
	}
	name := freeNameIn(tickets.ArchiveDir(dir), base, len(halves) > 1)
	var live []string
	for _, h := range halves {
		live = append(live, workspaces.LiveEpicSessions(a.d, h.key, allKinds)...)
	}
	out := wire.TargetResult{Target: target, Status: wire.TargetDone, Sessions: live, OnTrunk: &onTrunk}
	if name != base {
		out.NewAddress = project + ":" + name
		if asImpl {
			out.NewAddress += tickets.ImplEpicSuffix
		}
	}
	if dryRun {
		out.Status = wire.TargetFits
		return out, nil
	}
	if len(live) > 0 && !yes {
		r := wire.RefusedTarget(target, wire.ReasonNeedsYes,
			fmt.Sprintf("archiving stops the live sessions: %s; confirm to continue", strings.Join(live, ", ")))
		r.Sessions = live
		return r, nil
	}
	out.OnTrunk = nil
	// The Design cleanup refuses under a live Design Session, so it runs once
	// write has stopped the Sessions of every half and before the rename.
	var cleanup func()
	if len(halves) > 1 && onTrunk {
		cleanup = func() { a.d.CleanupDesign(project, base) }
	}
	if err := a.write(dir, halves, name, cleanup); err != nil {
		if r, ok := errors.AsType[refusal](err); ok {
			return refuse(r.reason, r.Error())
		}
		if f, ok := errors.AsType[failure](err); ok {
			failed := wire.FailedTarget(target, f)
			failed.NewAddress = out.NewAddress
			return failed, nil
		}
		return wire.TargetResult{}, err
	}
	return out, nil
}

const allKinds = workspaces.OwnSessions | workspaces.FixSessions | workspaces.DesignSessions

type refusal struct {
	reason string
	err    error
}

func (r refusal) Error() string { return r.err.Error() }
func (r refusal) Unwrap() error { return r.err }

// failure is an error after the move began: the epic moved but a later step
// did not finish, so the target reports failed instead of done.
type failure struct{ err error }

func (f failure) Error() string { return f.err.Error() }
func (f failure) Unwrap() error { return f.err }

// half is one epic of a row: a loop epic, a Design or its -impl epic.
type half struct{ epic, key string }

// freeNameIn is the epic's name, or the first free `-N` suffix when dir has
// it. A pair needs both `name` and `name-impl` free.
func freeNameIn(dir, epic string, pair bool) string {
	taken := func(name string) bool {
		names := []string{name}
		if pair {
			names = append(names, name+tickets.ImplEpicSuffix)
		}
		for _, n := range names {
			if _, err := os.Stat(filepath.Join(dir, n)); !os.IsNotExist(err) {
				return true
			}
		}
		return false
	}
	name := epic
	for n := 2; taken(name); n++ {
		name = epic + "-" + strconv.Itoa(n)
	}
	return name
}

// write runs the effects in order; the first half is the Design of a pair.
// Sessions the stop already ended stay stopped. cleanup, when set, runs after
// every Session is stopped and before the rename.
func (a *Archive) write(dir string, halves []half, name string, cleanup func()) error {
	for _, h := range halves {
		if _, err := workspaces.StopEpicSessions(a.d, h.key, allKinds); err != nil {
			return err
		}
		if err := a.d.Unhold(h.key); err != nil {
			return err
		}
	}
	if cleanup != nil {
		cleanup()
	}
	archive := tickets.ArchiveDir(dir)
	if err := os.MkdirAll(archive, 0o755); err != nil {
		return err
	}
	type move struct {
		half
		name, workspace, branch string
	}
	moves := make([]move, len(halves))
	for i, h := range halves {
		m := move{half: h, name: name, workspace: a.d.Workspace(h.key), branch: h.epic}
		if i > 0 {
			m.name += tickets.ImplEpicSuffix
		}
		if e, ok := a.d.Epic(h.key); ok {
			m.branch = cmp.Or(e.BranchName, h.epic)
		}
		moves[i] = m
	}
	for i, m := range moves {
		if err := os.Rename(filepath.Join(dir, m.epic), filepath.Join(archive, m.name)); err != nil {
			for _, done := range moves[:i] {
				_ = os.Rename(filepath.Join(archive, done.name), filepath.Join(dir, done.epic))
			}
			return refusal{wire.ReasonArchiveRenameFailed, err}
		}
	}
	var rekeyErrs []error
	for i, m := range moves {
		fields := map[string]any{"archived_at": time.Now().UTC()}
		if m.workspace != workspaces.DefaultName {
			fields["archived_workspace"] = m.workspace
		}
		// branch_name goes on the Design only; the -impl branch follows its name.
		if i == 0 && m.name != m.epic {
			fields["branch_name"] = m.branch
		}
		if err := tickets.OverwriteEpicFields(filepath.Join(archive, m.name), fields); err != nil {
			return err
		}
		rekeyErrs = append(rekeyErrs, a.rekey(tickets.EpicAddress, m.key, tickets.ArchiveKey(m.key, m.name)))
	}
	a.d.Events.Publish(wire.EventWorkspacesChanged, "")
	if err := errors.Join(rekeyErrs...); err != nil {
		return failure{err}
	}
	return nil
}

// rekey points every Session of the epic keyed `from`, live or exited, at the
// key `to`, and joins the errors of those it could not. epicOf cuts a ticket
// address down to its epic key; an archive key needs tickets.ArchiveEpicKey,
// as its first slash is not the ticket's.
func (a *Archive) rekey(epicOf func(string) string, from, to string) error {
	var errs []error
	rekeyOne := func(addr string) string {
		if epicOf(addr) == from {
			return to + strings.TrimPrefix(addr, from)
		}
		return addr
	}
	for _, si := range a.d.Sessions() {
		if epicOf(si.Ticket) != from && epicOf(si.FixOf) != from {
			continue
		}
		err := a.d.RekeySession(si.ID, func(ticket, fixOf string) (string, string) {
			return rekeyOne(ticket), rekeyOne(fixOf)
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("re-key session %s: %w", si.ID, err))
			continue
		}
		a.d.Events.Publish(wire.EventSessionChanged, si.ID)
	}
	return errors.Join(errs...)
}
