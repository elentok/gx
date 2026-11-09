// Package cleanup serves the cleanup scan of a project with each worktree's
// disk size. Sizes are read in the background and cached; the scan answers
// with the ones that are ready.
package cleanup

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/cleanupscan"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketgit"
)

const duTimeout = 10 * time.Minute

// Deps is what Cleanup reads from the Server.
type Deps struct {
	TicketStore string
	Env         []string
	Events      *broker.Broker
	// Size reads the bytes under path. Nil runs du.
	Size func(ctx context.Context, path string) (uint64, error)
	// Sessions lists every session, for refusing a worktree one still works in.
	Sessions func() ([]wire.SessionInfo, error)
	// EpicQueued reports whether the epic's root is in the ralph-loop queue.
	EpicQueued func(project, epic string) bool
	// SubmitOneOff starts a one-off agent ticket.
	SubmitOneOff func(wire.OneOffRequest) (wire.OneOffResult, error)
}

// Cleanup is the cleanup read route's state: the size cache.
type Cleanup struct {
	d Deps

	mu       sync.Mutex
	sizes    map[string]uint64
	inflight map[string]bool
	gen      int // bumped by Invalidate
}

// New builds Cleanup.
func New(d Deps) *Cleanup {
	c := &Cleanup{d: d, sizes: map[string]uint64{}, inflight: map[string]bool{}}
	if c.d.Size == nil {
		c.d.Size = c.du
	}
	return c
}

// Routes lists the cleanup read endpoint. The getter is called per request, so
// a route can be listed before its Cleanup is built.
func Routes(c func() *Cleanup) []wire.Route {
	return []wire.Route{
		{Pattern: "GET /v1/cleanup", Web: wire.WebRead, Handler: func(w http.ResponseWriter, r *http.Request) { c().read(w, r) }},
		{Pattern: "POST /v1/cleanup/remove-worktree", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) { c().removeWrite(w, r) }},
		{Pattern: "POST /v1/cleanup/remove-unmerged-worktree", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) { c().removeUnmergedWrite(w, r) }},
		{Pattern: "POST /v1/cleanup/archive-epic", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) { c().archiveWrite(w, r) }},
		{Pattern: "POST /v1/cleanup/agent", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) { c().agentWrite(w, r) }},
		{Pattern: "POST /v1/cleanup/delete-branch", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) { c().deleteWrite(w, r) }},
	}
}

func (c *Cleanup) read(w http.ResponseWriter, r *http.Request) {
	rep, err := c.Report(r.URL.Query().Get("project"))
	if err != nil {
		code := http.StatusInternalServerError
		if _, ok := errors.AsType[*tickets.AddressError](err); ok {
			code = http.StatusNotFound
		}
		http.Error(w, err.Error(), code)
		return
	}
	wire.WriteJSON(w, rep)
}

// Report scans project and starts a background size read for every worktree
// whose size is not cached yet.
func (c *Cleanup) Report(project string) (wire.CleanupReport, error) {
	dir, err := tickets.ProjectDirByName(c.d.TicketStore, project)
	if err != nil {
		return wire.CleanupReport{}, err
	}
	repo := ticketgit.ProjectRepoRoot(c.d.TicketStore, tickets.ProjectName(dir))
	if repo == "" {
		return wire.CleanupReport{}, fmt.Errorf("project %q has no repo", project)
	}
	res, err := cleanupscan.ScanIn(c.d.TicketStore, repo)
	if err != nil {
		return wire.CleanupReport{}, err
	}
	rep := wire.CleanupReport{Project: project, Epics: []wire.CleanupEpic{}, Worktrees: []wire.CleanupWorktree{}, AtticRefs: res.AtticRefs}
	for _, e := range res.Epics {
		rep.Epics = append(rep.Epics, wire.CleanupEpic{
			Name: e.Name, AllDone: e.AllDone, MergedToTrunk: e.MergedToTrunk,
			HasCodeReviewTicket: e.HasCodeReviewTicket, CodeReviewDone: e.CodeReviewDone,
			Action: epicAction(e),
		})
	}
	var missing []string
	for _, wt := range res.Worktrees {
		live := ""
		if wt.Path != "" {
			if live, err = c.liveSessionIn(wt.Path); err != nil {
				return wire.CleanupReport{}, err
			}
		}
		out := wire.CleanupWorktree{
			Branch: wt.Branch, Path: wt.Path, Kind: wt.Kind, Active: wt.Active, Dirty: wt.Dirty, Head: wt.Head,
			Epic: wt.Epic, TicketID: wt.TicketID, TicketDone: wt.TicketDone, Merged: wt.Merged,
			MergedToTrunk: wt.MergedToTrunk, DeleteMode: wt.DeleteMode, Recommendation: wt.Recommendation,
			Action: worktreeAction(wt, live != ""),
		}
		out.SizeBytes, missing = c.sizeOf(wt.Path, missing)
		rep.Worktrees = append(rep.Worktrees, out)
	}
	for _, p := range missing {
		go c.measure(project, p)
	}
	return rep, nil
}

// epicAction: a done epic archives once landed on trunk, else the agent decides it.
func epicAction(e cleanupscan.EpicScan) string {
	switch {
	case !e.AllDone:
		return ""
	case e.MergedToTrunk:
		return wire.CleanupActionArchive
	}
	return wire.CleanupActionAgent
}

// worktreeAction decides the row's one button, once, for the page and the agent
// prompt. A proven merge ignores Active and TicketDone, as the write routes do.
// Detached rows get none: the write routes find rows by branch.
func worktreeAction(wt cleanupscan.WorktreeScan, live bool) string {
	switch {
	case wt.Branch == "":
		return ""
	case wt.DeleteMode != cleanupscan.DeleteNone && wt.Path == "":
		return wire.CleanupActionMergedDelete
	case wt.DeleteMode != cleanupscan.DeleteNone:
		return wire.CleanupActionMergedRemove
	case wt.Path != "" && !wt.Dirty && !live:
		return wire.CleanupActionUnmergedRemove
	case wt.Path != "" || wt.Active:
		return ""
	}
	return wire.CleanupActionAgent
}

// sizeOf returns path's cached size, or queues path to be measured.
func (c *Cleanup) sizeOf(path string, missing []string) (*uint64, []string) {
	if path == "" {
		return nil, missing
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if n, ok := c.sizes[path]; ok {
		return &n, missing
	}
	if !c.inflight[path] {
		c.inflight[path] = true
		missing = append(missing, path)
	}
	return nil, missing
}

// Invalidate drops the cached sizes, so the next read measures again. A removal
// calls it.
func (c *Cleanup) Invalidate() {
	c.mu.Lock()
	c.sizes = map[string]uint64{}
	c.gen++
	c.mu.Unlock()
}

func (c *Cleanup) measure(project, path string) {
	ctx, cancel := context.WithTimeout(context.Background(), duTimeout)
	defer cancel()
	c.mu.Lock()
	gen := c.gen
	c.mu.Unlock()
	n, err := c.d.Size(ctx, path)
	c.mu.Lock()
	delete(c.inflight, path)
	// A size measured before an Invalidate may predate the removal.
	if err == nil && gen == c.gen {
		c.sizes[path] = n
	}
	c.mu.Unlock()
	if err == nil && c.d.Events != nil {
		c.d.Events.Publish(wire.EventCleanupSizeChanged, project)
	}
}

func (c *Cleanup) du(ctx context.Context, path string) (uint64, error) {
	cmd := exec.CommandContext(ctx, "du", "-sk", path)
	cmd.Env = c.d.Env
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	out, err := cmd.Output()
	// du exits non-zero on an unreadable file but still prints the total.
	f := strings.Fields(string(out))
	if len(f) == 0 {
		if err == nil {
			err = errors.New("du printed nothing")
		}
		return 0, err
	}
	kb, perr := strconv.ParseUint(f[0], 10, 64)
	if perr != nil {
		return 0, perr
	}
	return kb * 1024, nil
}
