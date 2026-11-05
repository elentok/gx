package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

func (rs *Resumer) sessionLinkWrite(w http.ResponseWriter, r *http.Request) {
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
		return sessionRefusal(wire.ReasonSessionBadRequest, "a link needs a ticket address"), nil
	}
	dir, meta, res, err := rs.mg.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	ticket := req.Ticket
	if !strings.Contains(ticket, ":") {
		ticket = meta.Project + ":" + ticket
	}
	design := rs.mg.onMapEpic(ticket)
	var info wire.SessionInfo
	if err := rs.mg.updateSessionMeta(dir, req.ID, func(m *sessionMeta) bool {
		m.Ticket, m.Design = ticket, m.Design || design
		rs.mg.recordConversation(*m)
		info = m.info()
		return true
	}); err != nil {
		return wire.SessionResult{}, err
	}
	return wire.SessionResult{Session: &info}, nil
}

func (rs *Resumer) sessionStepWrite(w http.ResponseWriter, r *http.Request) {
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
		return sessionRefusal(wire.ReasonSessionBadRequest, "a step report needs a step"), nil
	}
	dir, meta, res, err := rs.mg.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if meta.Status.State == wire.SessionExited {
		return sessionRefusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s has exited", req.ID)), nil
	}
	var info wire.SessionInfo
	if err := rs.mg.updateSessionMeta(dir, req.ID, func(m *sessionMeta) bool {
		m.ReportedStep, m.ReportedTitle = req.Step, req.Title
		info = m.info()
		return true
	}); err != nil {
		return wire.SessionResult{}, err
	}
	return wire.SessionResult{Session: &info}, nil
}

// stampDesignStart records the first design session on the map epic behind
// address. The stamp is write-once, so every later session leaves it.
func (mg *Manager) stampDesignStart(address string, now time.Time) error {
	epicDir, ok, err := mg.epicDir(address)
	if err != nil || !ok {
		return err
	}
	return tickets.StampEpicDesignStarted(epicDir, now)
}

// epicDir resolves address ("project:epic" or "project:epic/NN") to its epic
// directory; ok is false when the address names no epic.
func (mg *Manager) epicDir(address string) (dir string, ok bool, err error) {
	project, rest, found := strings.Cut(address, ":")
	epic, _, _ := strings.Cut(rest, "/")
	if !found || epic == "" {
		return "", false, nil
	}
	projectDir, err := mg.projects.Dir(project)
	if err != nil {
		return "", false, err
	}
	return filepath.Join(projectDir, epic), true, nil
}

// designLaunchTier is the tier of a design-launched session on the map behind
// address: strongest while charting or while the destination is unconfirmed,
// worker at every later step. An unreadable map charts.
func (mg *Manager) designLaunchTier(address string) ralphloop.Tier {
	dir, ok, err := mg.epicDir(address)
	if err != nil || !ok {
		return ralphloop.TierStrongest
	}
	m := tickets.LoadEpicTicketMD(dir)
	if m.DestinationUnconfirmed || m.DesignStep == "" || m.DesignStep == string(tickets.StepChart) {
		return ralphloop.TierStrongest
	}
	return ralphloop.TierWorker
}

// onMapEpic reports whether address ("project:epic" or "project:epic/NN")
// is on a map epic; an unknown project or epic is not.
func (mg *Manager) onMapEpic(address string) bool {
	dir, ok, err := mg.epicDir(address)
	return err == nil && ok && tickets.IsMapEpic(dir)
}
