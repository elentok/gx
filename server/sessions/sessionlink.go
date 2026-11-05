package sessions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

func (rs *Resumer) SessionLinkWrite(w http.ResponseWriter, r *http.Request) {
	var req wire.LinkSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := rs.linkSession(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

// linkSession sets the session's Session ticket, qualifying a bare address
// with the session's project, and marks it Design when the ticket is on a
// map epic. An existing Design flag stays: a new design session links to an
// epic Chart has not written yet.
func (rs *Resumer) linkSession(req wire.LinkSessionRequest) (wire.SessionResult, error) {
	if req.Ticket == "" {
		return Refusal(wire.ReasonSessionBadRequest, "a link needs a ticket address"), nil
	}
	dir, meta, res, err := rs.mg.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	ticket := req.Ticket
	if !strings.Contains(ticket, ":") {
		ticket = meta.Project + ":" + ticket
	}
	design := rs.mg.Maps.OnMapEpic(ticket)
	var info wire.SessionInfo
	if err := rs.mg.UpdateSessionMeta(dir, req.ID, func(m *Meta) bool {
		m.Ticket, m.Design = ticket, m.Design || design
		rs.mg.Conv.Record(m.ConvSession())
		info = m.Info()
		return true
	}); err != nil {
		return wire.SessionResult{}, err
	}
	return wire.SessionResult{Session: &info}, nil
}

func (rs *Resumer) SessionStepWrite(w http.ResponseWriter, r *http.Request) {
	var req wire.ReportStepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := rs.reportSessionStep(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

// reportSessionStep saves the Design step an agent says it is running,
// replacing any earlier report.
func (rs *Resumer) reportSessionStep(req wire.ReportStepRequest) (wire.SessionResult, error) {
	if req.Step == "" {
		return Refusal(wire.ReasonSessionBadRequest, "a step report needs a step"), nil
	}
	dir, meta, res, err := rs.mg.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if meta.Status.State == wire.SessionExited {
		return Refusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s has exited", req.ID)), nil
	}
	var info wire.SessionInfo
	if err := rs.mg.UpdateSessionMeta(dir, req.ID, func(m *Meta) bool {
		m.ReportedStep, m.ReportedTitle = req.Step, req.Title
		info = m.Info()
		return true
	}); err != nil {
		return wire.SessionResult{}, err
	}
	return wire.SessionResult{Session: &info}, nil
}

// MapEpics answers what the sessions need to know about the map epic behind a
// ticket address.
type MapEpics struct{ projects *projects.Registry }

// stampDesignStart records the first design session on the map epic behind
// address. The stamp is write-once, so every later session leaves it.
func (me *MapEpics) stampDesignStart(address string, now time.Time) error {
	epicDir, ok, err := me.epicDir(address)
	if err != nil || !ok {
		return err
	}
	return tickets.StampEpicDesignStarted(epicDir, now)
}

// epicDir resolves address ("project:epic" or "project:epic/NN") to its epic
// directory; ok is false when the address names no epic.
func (me *MapEpics) epicDir(address string) (dir string, ok bool, err error) {
	project, rest, found := strings.Cut(address, ":")
	epic, _, _ := strings.Cut(rest, "/")
	if !found || epic == "" {
		return "", false, nil
	}
	projectDir, err := me.projects.Dir(project)
	if err != nil {
		return "", false, err
	}
	return filepath.Join(projectDir, epic), true, nil
}

// DesignLaunchTier is the tier of a design-launched session on the map behind
// address: strongest while charting or while the destination is unconfirmed,
// worker at every later step. An unreadable map charts.
func (me *MapEpics) DesignLaunchTier(address string) ralphloop.Tier {
	dir, ok, err := me.epicDir(address)
	if err != nil || !ok {
		return ralphloop.TierStrongest
	}
	m := tickets.LoadEpicTicketMD(dir)
	if m.DestinationUnconfirmed || m.DesignStep == "" || m.DesignStep == string(tickets.StepChart) {
		return ralphloop.TierStrongest
	}
	return ralphloop.TierWorker
}

// OnMapEpic reports whether address ("project:epic" or "project:epic/NN")
// is on a map epic; an unknown project or epic is not.
func (me *MapEpics) OnMapEpic(address string) bool {
	dir, ok, err := me.epicDir(address)
	return err == nil && ok && tickets.IsMapEpic(dir)
}
