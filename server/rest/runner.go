package rest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/wire"
)

const stateFileName = "rest.json"

// Runner is the live Rest engine: the core's state, the settings, the clock,
// the files and the timers that move a Rest period without a request.
type Runner struct {
	mu      sync.Mutex
	path    string
	records *Records
	events  *broker.Broker
	cfg     func() config.MentalHealthConfig
	now     func() time.Time
	state   State
	view    View
	timer   *time.Timer
	started bool
	closed  bool
}

// Open loads the persisted state from stateDir. cfg is read on every call, so
// a settings change needs no copy. It reads no settings until Start.
func Open(stateDir string, events *broker.Broker, cfg func() config.MentalHealthConfig, now func() time.Time) (*Runner, error) {
	records, err := OpenRecords(stateDir, now)
	if err != nil {
		return nil, err
	}
	r := &Runner{path: filepath.Join(stateDir, stateFileName), records: records, events: events, cfg: cfg, now: now}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

// Records is the store of Break starts, dropped Breaks and unlocks.
func (r *Runner) Records() *Records { return r.records }

// Start brings the loaded state up to date with the settings and the clock
// and arms the timers.
func (r *Runner) Start() error {
	r.mu.Lock()
	r.started = true
	r.mu.Unlock()
	_, err := r.apply(Event{Kind: EventTick})
	return err
}

// Close stops the timers.
func (r *Runner) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.stopTimerLocked()
}

// View is the current view. It changes nothing.
func (r *Runner) View() View {
	r.mu.Lock()
	defer r.mu.Unlock()
	return Step(Input{State: r.state, Config: r.cfg(), Now: r.now(), Event: Event{Kind: EventTick}}).View
}

// Quiet is the one predicate Banner, sound and Chat ask: see State.Quiet.
func (r *Runner) Quiet() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state.Quiet(r.cfg(), r.now())
}

// Active counts one active minute.
func (r *Runner) Active() (Result, error) { return r.apply(Event{Kind: EventActive}) }

// Unlock tries to end the running Rest period with the typed sentence. A wrong
// sentence comes back as Result.Refusal, not as an error.
func (r *Runner) Unlock(sentence string) (Result, error) {
	return r.apply(Event{Kind: EventUnlock, Sentence: sentence})
}

// SettingsChanged delivers a settings change; the core ends what no longer holds.
func (r *Runner) SettingsChanged() (Result, error) { return r.apply(Event{Kind: EventSettings}) }

// Tick is the passage of time. A timer calls it, and so does a test.
func (r *Runner) Tick() (Result, error) { return r.apply(Event{Kind: EventTick}) }

func (r *Runner) apply(ev Event) (Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	res := Step(Input{State: r.state, Config: r.cfg(), Now: r.now(), Event: ev})
	changed := !reflect.DeepEqual(r.state, res.State) || !sameView(r.view, res.View)
	stateChanged := !reflect.DeepEqual(r.state, res.State)
	r.state, r.view = res.State, res.View
	var err error
	if stateChanged {
		err = r.save()
	}
	err = errors.Join(err, r.records.Append(res.Records...))
	r.scheduleLocked()
	if changed {
		r.events.Publish(wire.EventRestChanged, "")
	}
	return res, err
}

// sameView compares what clients see, ignoring the time the view was made for.
func sameView(a, b View) bool {
	a.Now, b.Now = time.Time{}, time.Time{}
	return reflect.DeepEqual(a, b)
}

func (r *Runner) stopTimerLocked() {
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
}

func (r *Runner) scheduleLocked() {
	r.stopTimerLocked()
	if r.closed || !r.started {
		return
	}
	now := r.now()
	if at, ok := NextWake(r.cfg(), r.state, now); ok {
		r.timer = time.AfterFunc(at.Sub(now), func() { _, _ = r.Tick() })
	}
}

type persistedPeriod struct {
	Kind       Kind      `json:"kind"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	Message    string    `json:"message,omitempty"`
	SubMessage string    `json:"sub_message,omitempty"`
}

type persisted struct {
	WorkCount    int              `json:"work_count"`
	LastActive   time.Time        `json:"last_active,omitzero"`
	Period       *persistedPeriod `json:"period,omitempty"`
	QuietUntil   time.Time        `json:"quiet_until,omitzero"`
	UnlockedKind Kind             `json:"unlocked_kind,omitempty"`
}

// save writes through a rename so a crash never leaves a torn file.
func (r *Runner) save() error {
	p := persisted{
		WorkCount: r.state.WorkCount, LastActive: r.state.LastActive,
		QuietUntil: r.state.QuietUntil, UnlockedKind: r.state.UnlockedKind,
	}
	if pe := r.state.Period; pe != nil {
		p.Period = &persistedPeriod{Kind: pe.Kind, Start: pe.Start, End: pe.End, Message: pe.Message, SubMessage: pe.SubMessage}
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}

// load reads the persisted state. Times come back in the clock's location, as
// Bedtime is read in the location of the times the core is given.
func (r *Runner) load() error {
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var p persisted
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	loc := r.now().Location()
	r.state = State{
		WorkCount: p.WorkCount, LastActive: p.LastActive.In(loc),
		QuietUntil: p.QuietUntil.In(loc), UnlockedKind: p.UnlockedKind,
	}
	if pe := p.Period; pe != nil {
		r.state.Period = &Period{Kind: pe.Kind, Start: pe.Start.In(loc), End: pe.End.In(loc), Message: pe.Message, SubMessage: pe.SubMessage}
	}
	return nil
}
