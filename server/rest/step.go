package rest

import (
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/wire"
)

const (
	bedtimeWarnLead = 10 * time.Minute
	// breakWarnMinutes is how far below Work time the count must be for the
	// Break warning to appear.
	breakWarnMinutes = 2
)

var unlockSentence = regexp.MustCompile(`(?i)^\s*i['’]m\s+.+?\s+and\s+i['’]m\s+addicted\s+to\s+ai\s*$`)

// Step applies one event. Every event first brings the state up to date with
// the settings and the time, so a settings change and a tick need no code of
// their own beyond that.
func Step(in Input) Result {
	e := &engine{s: in.State, cfg: in.Config, now: in.Now, rand: in.Rand}
	if e.rand == nil {
		e.rand = rand.IntN
	}
	e.reconcile()
	var refusal string
	switch in.Event.Kind {
	case EventActive:
		e.active()
	case EventUnlock:
		refusal = e.unlock(in.Event.Sentence)
	}
	return Result{State: e.s, Records: e.records, View: e.view(), Refusal: refusal}
}

type engine struct {
	s       State
	cfg     config.MentalHealthConfig
	now     time.Time
	rand    func(int) int
	records []Record
}

// reconcile ends what no longer holds and starts Bedtime when it is due.
func (e *engine) reconcile() {
	b := e.cfg.Bedtime
	if p := e.s.Period; p != nil {
		switch {
		case !e.cfg.Enabled,
			p.Kind == KindBreak && (!e.cfg.Break.Enabled || !e.now.Before(p.End)),
			p.Kind == KindBedtime && (!b.Enabled || !e.now.Before(p.End) || !inBedtime(b, e.now)):
			e.s.Period = nil
			e.s.WorkCount = 0
		}
	}
	if !e.cfg.Enabled || !e.cfg.Break.Enabled {
		e.s.WorkCount = 0
	}
	idle := time.Duration(e.cfg.Break.BreakMinutes) * time.Minute
	if !e.s.LastActive.IsZero() && e.now.Sub(e.s.LastActive) >= idle {
		e.s.WorkCount = 0
	}
	if e.s.UnlockedKind != "" && !e.now.Before(e.s.QuietUntil) {
		e.s.UnlockedKind = ""
		e.s.QuietUntil = time.Time{}
	}
	if e.cfg.Enabled && b.Enabled && inBedtime(b, e.now) && !bedtimeUnlocked(e.s, e.now) &&
		(e.s.Period == nil || e.s.Period.Kind == KindBreak) {
		e.startBedtime()
	}
}

func bedtimeUnlocked(s State, now time.Time) bool {
	return s.UnlockedKind == KindBedtime && now.Before(s.QuietUntil)
}

// startBedtime drops a running Break, with a record that cancels its start.
func (e *engine) startBedtime() {
	if p := e.s.Period; p != nil {
		e.records = append(e.records, Record{Kind: RecordBreakDropped, Time: e.now, Started: p.Start})
	}
	e.s.WorkCount = 0
	e.s.Period = &Period{Kind: KindBedtime, Start: e.now, End: nextClock(e.now, e.cfg.Bedtime.End)}
}

// counting is true while an active minute would count.
func (e *engine) counting() bool {
	return e.cfg.Enabled && e.cfg.Break.Enabled && e.s.Period == nil && !bedtimeUnlocked(e.s, e.now)
}

func (e *engine) active() {
	if !e.counting() {
		return
	}
	minute := e.now.Truncate(time.Minute)
	if minute.Equal(e.s.LastActive) {
		return
	}
	e.s.LastActive = minute
	e.s.WorkCount++
	if e.s.WorkCount >= e.cfg.Break.WorkMinutes {
		e.startBreak()
	}
}

func (e *engine) startBreak() {
	msg, sub := e.breakText()
	e.s.Period = &Period{
		Kind:       KindBreak,
		Start:      e.now,
		End:        e.now.Add(time.Duration(e.cfg.Break.BreakMinutes) * time.Minute),
		Message:    msg,
		SubMessage: sub,
	}
	e.records = append(e.records, Record{Kind: RecordBreakStart, Time: e.now})
}

func (e *engine) breakText() (message, sub string) {
	if custom := strings.TrimSpace(e.cfg.Break.Message); custom != "" {
		return splitMessage(custom)
	}
	pair := builtinBreakMessages[e.rand(len(builtinBreakMessages))]
	work := strconv.Itoa(e.cfg.Break.WorkMinutes)
	return strings.ReplaceAll(pair.message, "{work}", work), strings.ReplaceAll(pair.sub, "{work}", work)
}

// unlock ends the running Rest period and stays quiet until its original end.
func (e *engine) unlock(sentence string) string {
	p := e.s.Period
	if p == nil {
		return wire.ReasonRestNoPeriod
	}
	if !unlockSentence.MatchString(sentence) {
		return wire.ReasonRestSentenceMismatch
	}
	e.records = append(e.records, Record{Kind: RecordUnlock, Time: e.now, Rest: p.Kind})
	e.s.QuietUntil = p.End
	e.s.UnlockedKind = p.Kind
	e.s.Period = nil
	e.s.WorkCount = 0
	return ""
}

func (e *engine) view() View {
	v := View{
		Now:      e.now,
		Counting: e.counting(),
		Warning:  e.warning(),
		quiet:    e.s.Quiet(e.cfg, e.now),
	}
	if p := e.s.Period; p != nil {
		pv := *p
		if p.Kind == KindBedtime {
			text := strings.ReplaceAll(e.cfg.Bedtime.Message, "{end}", e.cfg.Bedtime.End)
			pv.Message, pv.SubMessage = splitMessage(text)
		}
		v.Period = &pv
	}
	return v
}

// nextBedtimeStart is when Bedtime next starts, after any unlocked Bedtime.
func nextBedtimeStart(cfg config.MentalHealthConfig, s State, now time.Time) (time.Time, bool) {
	if !cfg.Enabled || !cfg.Bedtime.Enabled {
		return time.Time{}, false
	}
	from := now
	if bedtimeUnlocked(s, now) {
		from = s.QuietUntil
	}
	return nextClock(from, cfg.Bedtime.Start), true
}

// warning picks the one warning to show. A Break warning only exists when its
// start is before the next Bedtime start, so it is always the earlier one.
func (e *engine) warning() *Warning {
	bedStart, hasBed := nextBedtimeStart(e.cfg, e.s, e.now)
	if w := e.breakWarning(bedStart, hasBed); w != nil {
		return w
	}
	if hasBed && (e.s.Period == nil || e.s.Period.Kind == KindBreak) && !e.now.Before(bedStart.Add(-bedtimeWarnLead)) {
		return &Warning{Kind: KindBedtime, Start: bedStart}
	}
	return nil
}

// breakWarning predicts the start assuming the person stays active every
// minute from now on, so an idle minute moves it later.
func (e *engine) breakWarning(bedStart time.Time, hasBed bool) *Warning {
	left := e.cfg.Break.WorkMinutes - e.s.WorkCount
	if !e.counting() || e.s.WorkCount == 0 || left > breakWarnMinutes {
		return nil
	}
	from := e.s.LastActive
	if prev := e.now.Truncate(time.Minute).Add(-time.Minute); prev.After(from) {
		from = prev
	}
	start := from.Add(time.Duration(left) * time.Minute)
	if hasBed && !start.Before(bedStart) {
		return nil
	}
	return &Warning{Kind: KindBreak, Start: start, BreakMinutes: e.cfg.Break.BreakMinutes}
}
