package sessions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/elentok/gx/modelrules"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/wire"
)

// switchState is a hand switch whose new agent is not idle yet. run is the run
// that agent will have.
type switchState struct {
	run           int
	model, effort string
	// back marks the relaunch on the old record after a failed switch; model
	// is then the old model and attempted the one that failed.
	back      bool
	attempted string
}

// switchFailure is the outcome of a failed switch, shown until the next one.
type switchFailure struct{ attempted, old string }

// switchStates keeps the running switches in memory only, so a crash leaves
// no trace of one and the record stays the old one.
type switchStates struct {
	mu     sync.Mutex
	m      map[string]switchState
	failed map[string]switchFailure
}

func (s *switchStates) failure(id string) (switchFailure, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.failed[id]
	return f, ok
}

// setFailure records a failed switch; the zero switchFailure clears it.
func (s *switchStates) setFailure(id string, f switchFailure) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f == (switchFailure{}) {
		delete(s.failed, id)
		return
	}
	if s.failed == nil {
		s.failed = map[string]switchFailure{}
	}
	s.failed[id] = f
}

func (s *switchStates) set(id string, st switchState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]switchState{}
	}
	s.m[id] = st
}

func (s *switchStates) get(id string) (switchState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.m[id]
	return st, ok
}

// take removes and returns the switch of id when its new agent is run.
func (s *switchStates) take(id string, run int) (switchState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.m[id]
	if !ok || st.run != run {
		return switchState{}, false
	}
	delete(s.m, id)
	return st, true
}

func (s *switchStates) clear(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, id)
}

// SwitchSession relaunches an idle session's conversation with a new model and
// effort. gx checks neither: the harness decides what is valid. It stops the old
// agent without marking the session stopped and resumes the same conversation
// under the new flags. The record keeps the old model and effort until the new
// agent is idle (see watchSession), so a crash before that resumes on the old
// flags. A new agent that exits first ends the session as a failed start does.
func (rs *Resumer) SwitchSession(req wire.SessionSwitchRequest) (wire.SessionResult, error) {
	m := rs.mg
	dir, res, err := rs.beginSwitch(req)
	if res.Refused || err != nil {
		return res, err
	}
	rs.stopForRestart(dir, req.ID)
	m.Watches.MetaMu.Lock()
	defer m.Watches.MetaMu.Unlock()
	meta, err := ReadMeta(dir)
	if err != nil {
		m.switches.clear(req.ID)
		return wire.SessionResult{}, fmt.Errorf("switch session %s: %w", req.ID, err)
	}
	if meta.Status.State == wire.SessionExited {
		m.switches.clear(req.ID)
		return Refusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s ended during the switch", req.ID)), nil
	}
	launch := meta
	m.RecordSwitch(&launch, req.Model, req.Effort)
	if err := rs.startResumedHolderAs(dir, &meta, &launch, time.Time{}); err != nil {
		m.log.Warn("switch: new agent did not start", "session", req.ID, "err", err)
		if err := rs.rollbackSwitchLocked(dir, &meta, req.Model); err != nil {
			return wire.SessionResult{}, err
		}
	}
	info := m.SessionInfo(meta)
	return wire.SessionResult{Session: &info}, nil
}

// rollbackSwitchLocked relaunches the same conversation on the record as it
// stands, which a switch only changes once its agent is idle, and notes that
// the switch to attempted failed; the outcome shows once the old agent is
// idle. A relaunch that fails too ends the session as a failed start does.
// Switches 10 and 11 call it from their own triggers. The caller holds metaMu.
func (rs *Resumer) rollbackSwitchLocked(dir string, meta *Meta, attempted string) error {
	m := rs.mg
	m.switches.clear(meta.ID)
	m.switches.set(meta.ID, switchState{run: meta.Run + 1, model: meta.Model, effort: meta.Effort, back: true, attempted: attempted})
	if err := rs.startResumedHolderAs(dir, meta, meta, time.Time{}); err != nil {
		m.switches.clear(meta.ID)
		m.endFailedStart(dir, meta)
		return err
	}
	return nil
}

// failSwitch handles the exit of a switched agent that never reached idle. The
// old record is relaunched; an exit of that relaunch ends the session crashed.
func (rs *Resumer) failSwitch(dir, id string, run int, sw switchState) {
	mg := rs.mg
	if mg.Watches.IsStopped() {
		return
	}
	mg.Watches.MetaMu.Lock()
	defer mg.Watches.MetaMu.Unlock()
	meta, err := ReadMeta(dir)
	if err != nil || meta.Run != run || meta.Status.State == wire.SessionExited {
		return
	}
	if sw.back {
		mg.endFailedStart(dir, &meta)
		return
	}
	if err := rs.rollbackSwitchLocked(dir, &meta, sw.model); err != nil {
		mg.log.Warn("switch: old agent did not start", "session", id, "err", err)
	}
}

// beginSwitch refuses a session that cannot switch. Otherwise it bumps the run,
// which makes the old watch's exit a no-op, and notes the switch, all under one
// lock so a second switch sees a starting session.
func (rs *Resumer) beginSwitch(req wire.SessionSwitchRequest) (string, wire.SessionResult, error) {
	m := rs.mg
	m.Watches.MetaMu.Lock()
	defer m.Watches.MetaMu.Unlock()
	dir, meta, res, err := m.SessionDir(req.ID)
	if res.Refused || err != nil {
		return "", res, err
	}
	if req.Model == "" {
		return "", Refusal(wire.ReasonSessionBadRequest, "model is required"), nil
	}
	if !m.settings.Load().HasChatReader(meta.Harness) {
		return "", Refusal(wire.ReasonSessionUnknownHarness, fmt.Sprintf("harness %s cannot switch models", meta.Harness)), nil
	}
	if meta.Status.State == wire.SessionExited {
		return "", Refusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s is not live", req.ID)), nil
	}
	if _, switching := m.switches.get(req.ID); switching || meta.Status.State != wire.SessionIdle {
		return "", Refusal(wire.ReasonSessionNotIdle, fmt.Sprintf("session %s is %s, not idle", req.ID, meta.Info().Status.State)), nil
	}
	if _, err := holder.DialHolder(dir); err != nil {
		return "", Refusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s has no live agent", req.ID)), nil
	}
	meta.Run++
	if err := WriteMeta(dir, meta); err != nil {
		return "", wire.SessionResult{}, fmt.Errorf("switch session %s: %w", req.ID, err)
	}
	m.switches.setFailure(req.ID, switchFailure{})
	m.switches.set(req.ID, switchState{run: meta.Run + 1, model: req.Model, effort: req.Effort})
	// An accepted switch stops both countdowns, as typing does; a refused one
	// returned above and left them running.
	const why = "Countdown stopped: you switched the model"
	m.prompter.CancelAutoAdvance(req.ID, why)
	m.prompter.CancelAutoQueue(req.ID, why)
	return dir, wire.SessionResult{}, nil
}

// SessionSwitchWrite serves POST /v1/sessions/switch. The web mux also checks
// the state token and refuses a Queue agent.
func (rs *Resumer) SessionSwitchWrite(web bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req wire.SessionSwitchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := wire.SessionResult{}, error(nil)
		if web {
			res, err = rs.mg.checkInterruptWrite(wire.SessionRequest{ID: req.ID, State: req.State})
		}
		if err == nil && !res.Refused {
			res, err = rs.SwitchSession(req)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		wire.WriteJSON(w, res)
	}
}

// RecordSwitch applies a successful hand switch to meta with the session's
// harness, project and tier as configured now. It does not write the file: the
// caller does once the new agent is alive.
func (mg *Manager) RecordSwitch(meta *Meta, model, effort string) {
	tierModel, _ := agent.ResolveTier(mg.projects.AgentsConfig(meta.Project), agent.AgentFor(meta.Harness), meta.Tier)
	list, _ := mg.chat.Models().Entries(meta.Harness)
	RecordSwitch(meta, model, effort, tierModel, list, time.Now())
}

// RecordSwitch sets the new model and effort and keeps the tier. The by-hand
// flag is set unless model is tierModel, the model configured for the tier; a
// Claude alias equals its resolved id in list, and a session with no tier
// always keeps the flag set. The rollback target is the record as it stands,
// except that a switch still unconfirmed keeps its earlier target, so a second
// switch rolls back to the last confirmed record.
func RecordSwitch(meta *Meta, model, effort, tierModel string, list []modelrules.Entry, now time.Time) {
	if !meta.SwitchUnconfirmed {
		meta.SwitchPrev = &SwitchTarget{Model: meta.Model, Effort: meta.Effort, ByHand: meta.SwitchedByHand}
	}
	meta.SwitchUnconfirmed = true
	meta.SwitchedAt = now
	meta.Model, meta.Effort = model, effort
	meta.SwitchedByHand = meta.Tier == "" || !sameModel(list, tierModel, model)
}

// sameModel reports whether two configured models are one, comparing a launch
// value with a resolved id through list.
func sameModel(list []modelrules.Entry, a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return a == b || resolvedModelID(list, a) == resolvedModelID(list, b)
}

// resolvedModelID is the resolved id of a launch value, or the value itself
// when the list does not know it.
func resolvedModelID(list []modelrules.Entry, value string) string {
	for _, e := range list {
		if e.Value == value && e.ID != "" {
			return e.ID
		}
	}
	return value
}
