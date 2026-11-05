package server

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/server/epics"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/skills"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// rootRoutes serves the endpoints that belong to no subsystem: the handshake,
// the snapshot, the event stream and the restart routes. It is its own type so
// the handlers stay off Server.
type rootRoutes struct{ s *Server }

// routes lists them. The web mux serves the reads, and the restart write with
// the socket handler.
func (r rootRoutes) routes() []wire.Route {
	return []wire.Route{
		{Pattern: "GET /v1/handshake", Handler: r.handshake},
		{Pattern: "GET /v1/snapshot", Handler: r.snapshot, Web: wire.WebRead},
		{Pattern: "GET /v1/events", Handler: r.streamEvents, Web: wire.WebRead},
		{Pattern: "GET /v1/restart/check", Handler: r.restartCheck, Web: wire.WebRead},
		{Pattern: "POST /v1/restart", Handler: r.restartWrite, Web: wire.WebWrite},
	}
}

func (r rootRoutes) restartCheck(w http.ResponseWriter, req *http.Request) {
	r.s.restarter.ServeCheck(w, req)
}

func (r rootRoutes) restartWrite(w http.ResponseWriter, req *http.Request) {
	r.s.restarter.ServeWrite(w, req)
}

func (r rootRoutes) snapshot(w http.ResponseWriter, _ *http.Request) {
	s := r.s
	w.Header().Set("Content-Type", "application/json")
	snap, agents := s.idx.SnapshotAgents()
	snap.HerdrText, snap.HerdrUnavailable = s.herdr.Banner()
	for i, t := range snap.Tickets {
		if addr, err := tickets.ParseAddress(t.Address, tickets.AddressContext{}); err == nil {
			snap.Tickets[i].HarnessLabel = s.harness.LabelFor(addr, agents[i], cmp.Or(t.RenderedStatus, t.Status))
		}
		switch t.Status {
		case string(schema.StatusClaimed):
			snap.Tickets[i].ClaimedAt, _ = s.registry.StartedAtOf(t.Address)
		case string(schema.StatusPaused), string(schema.StatusLegacyAnswer), string(schema.StatusLegacyRepair):
			snap.Tickets[i].Recovery = ticketwrite.RecoveryState(s.pauseHold.State(t.Address), t.Address, snap.Tickets)
		}
	}
	s.merger.FillEpicMergeTargets(snap.Tickets)
	snap.Build = s.cfg.Build
	snap.Instance = s.restarter.Instance()
	snap.MigrationFailures = s.MigrationFailures()
	snap.Migrating = s.migrator.Migrating()
	snap.Budget = s.budget.Status(time.Now())
	snap.ExtraUsage = s.budget.ExtraUsageOn()
	snap.Disks = s.disks.Listed()
	snap.Health = s.health.Current()
	snap.Pending = s.explain.Pending()
	snap.Epics = s.epics.EpicSummaries(snap.Tickets)
	snap.RecentlyFinished = epics.RecentlyFinished(s.idx.Loaded(), s.settings.Load().RecentlyFinished)
	snap.Mode = s.queue.Mode()
	snap.AFK = s.afk.IsOn()
	snap.ViMode = s.viMode.IsOn()
	snap.SkillsInstallProblem = skills.ReadProblem(skills.StatusPath(s.cfg.StateDir))
	// Checked on every snapshot: a PATH entry can vanish while the server
	// runs, like an fnm per-shell dir once its shell exits.
	_, nodeErr := brief.LookPath(s.cfg.Env, "node")
	snap.NodeMissing = nodeErr != nil
	sessions, err := s.queue.ListSessionsWithQueue()
	if err != nil {
		s.log.Warn("list sessions for snapshot", "err", err)
	}
	snap.Sessions = sessions
	_ = json.NewEncoder(w).Encode(snap)
}

// DefaultSSEHeartbeat is how long /v1/events may sit idle before it writes a
// comment line, so proxies and browsers do not drop a quiet stream.
const DefaultSSEHeartbeat = 15 * time.Second

// streamEvents is SSE: every event with seq > ?since, then live events until
// the client leaves or is dropped for lagging. 410 means re-snapshot. A
// ?client= id keeps that client's focus reports alive while the stream is.
// An idle stream gets a ":" comment line every SSEHeartbeat.
func (r rootRoutes) streamEvents(w http.ResponseWriter, req *http.Request) {
	s := r.s
	var since uint64
	if v := req.URL.Query().Get("since"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			http.Error(w, "bad since", http.StatusBadRequest)
			return
		}
		since = n
	}
	ch, cancel, err := s.events.Subscribe(since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	defer cancel()
	ctx := req.Context()
	if id := webauth.LoginIDFrom(ctx); id != "" {
		var untrack func()
		ctx, untrack = s.web.Track(ctx, id)
		defer untrack()
	}
	if client := req.URL.Query().Get("client"); client != "" {
		s.focus.Connect(client)
		defer s.focus.Disconnect(client)
	}
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	if flusher != nil {
		flusher.Flush()
	}
	interval := s.cfg.SSEHeartbeat
	if interval <= 0 {
		interval = DefaultSSEHeartbeat
	}
	beat := time.NewTicker(interval)
	defer beat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-beat.C:
			if _, err := io.WriteString(w, ": heartbeat\n\n"); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		case ev, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(ev)
			if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, ev.Type, data); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

func (r rootRoutes) handshake(w http.ResponseWriter, _ *http.Request) {
	s := r.s
	w.Header().Set("Content-Type", "application/json")
	h := wire.Handshake{APIVersion: wire.APIVersion, Build: s.cfg.Build, Pid: os.Getpid(), TCPAddr: s.TCPAddr(), WebURL: s.WebURL(), WebError: s.web.Err()}
	h.HerdrText, h.HerdrUnavailable = s.herdr.Banner()
	h.Migrating = s.migrator.Migrating()
	if s.web.Listener() != nil {
		var err error
		if h.WebSetupPending, _, err = s.web.Gate().SetupState(); err != nil {
			s.log.Error("web setup state unavailable", "err", err)
		}
	}
	_ = json.NewEncoder(w).Encode(h)
}
