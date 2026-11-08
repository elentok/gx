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
}

// Cleanup is the cleanup read route's state: the size cache.
type Cleanup struct {
	d Deps

	mu       sync.Mutex
	sizes    map[string]uint64
	inflight map[string]bool
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
		{Pattern: "POST /v1/cleanup/remove-unmerged-worktree", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) { c().write(w, r, c().RemoveUnmergedWorktree) }},
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
		rep.Epics = append(rep.Epics, wire.CleanupEpic(e))
	}
	var missing []string
	c.mu.Lock()
	for _, wt := range res.Worktrees {
		out := wire.CleanupWorktree{
			Branch: wt.Branch, Path: wt.Path, Kind: wt.Kind, Active: wt.Active, Dirty: wt.Dirty, Head: wt.Head,
			Epic: wt.Epic, TicketID: wt.TicketID, TicketDone: wt.TicketDone, Merged: wt.Merged,
			MergedToTrunk: wt.MergedToTrunk, DeleteMode: wt.DeleteMode, Recommendation: wt.Recommendation,
		}
		if wt.Path != "" {
			if n, ok := c.sizes[wt.Path]; ok {
				out.SizeBytes = &n
			} else if !c.inflight[wt.Path] {
				c.inflight[wt.Path] = true
				missing = append(missing, wt.Path)
			}
		}
		rep.Worktrees = append(rep.Worktrees, out)
	}
	c.mu.Unlock()
	for _, p := range missing {
		go c.measure(project, p)
	}
	return rep, nil
}

// Invalidate drops the cached sizes, so the next read measures again. A removal
// calls it.
func (c *Cleanup) Invalidate() {
	c.mu.Lock()
	c.sizes = map[string]uint64{}
	c.mu.Unlock()
}

func (c *Cleanup) measure(project, path string) {
	ctx, cancel := context.WithTimeout(context.Background(), duTimeout)
	defer cancel()
	n, err := c.d.Size(ctx, path)
	c.mu.Lock()
	delete(c.inflight, path)
	if err == nil {
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
