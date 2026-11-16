package rest

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
)

var testZone = time.FixedZone("test", 2*60*60)

// at is a time on the fixed test day, in the test zone.
func at(h, m, s int) time.Time {
	return time.Date(2026, 11, 16, h, m, s, 0, testZone)
}

func atMin(m int) time.Time { return at(10, m, 0) }

func breakConfig() config.MentalHealthConfig {
	cfg := config.DefaultMentalHealthConfig()
	cfg.Enabled = true
	cfg.Break = config.MentalHealthBreak{Enabled: true, WorkMinutes: 10, BreakMinutes: 3}
	return cfg
}

func bedtimeConfig(start, end string) config.MentalHealthConfig {
	cfg := config.DefaultMentalHealthConfig()
	cfg.Enabled = true
	cfg.Bedtime.Enabled = true
	cfg.Bedtime.Start, cfg.Bedtime.End = start, end
	return cfg
}

// rig carries the state between steps like the runner will.
type rig struct {
	t       *testing.T
	state   State
	cfg     config.MentalHealthConfig
	rng     *rand.Rand
	records []Record
	last    Result
}

func newRig(t *testing.T, cfg config.MentalHealthConfig) *rig {
	return &rig{t: t, cfg: cfg, rng: rand.New(rand.NewPCG(1, 2))}
}

func (r *rig) step(now time.Time, ev Event) Result {
	r.t.Helper()
	res := Step(Input{State: r.state, Config: r.cfg, Now: now, Event: ev, Rand: r.rng.IntN})
	r.state, r.last = res.State, res
	r.records = append(r.records, res.Records...)
	return res
}

func (r *rig) active(now time.Time) Result { return r.step(now, Event{Kind: EventActive}) }
func (r *rig) tick(now time.Time) Result   { return r.step(now, Event{}) }
func (r *rig) unlock(now time.Time, s string) Result {
	return r.step(now, Event{Kind: EventUnlock, Sentence: s})
}

// settings applies a new config and reports it to the engine.
func (r *rig) settings(now time.Time, cfg config.MentalHealthConfig) Result {
	r.cfg = cfg
	return r.step(now, Event{Kind: EventSettings})
}

// activeMinutes sends one heartbeat in each of n minutes starting at minute from.
func (r *rig) activeMinutes(from, n int) {
	r.t.Helper()
	for i := range n {
		r.active(atMin(from + i))
	}
}

func recordKinds(recs []Record) []RecordKind {
	var out []RecordKind
	for _, rec := range recs {
		out = append(out, rec.Kind)
	}
	return out
}

func TestActive_TwoHeartbeatsInOneMinuteCountOnce(t *testing.T) {
	r := newRig(t, breakConfig())
	r.active(at(10, 0, 5))
	r.active(at(10, 0, 50))
	if r.state.WorkCount != 1 {
		t.Fatalf("count = %d, want 1", r.state.WorkCount)
	}
	r.active(at(10, 1, 1))
	if r.state.WorkCount != 2 {
		t.Fatalf("count = %d, want 2", r.state.WorkCount)
	}
}

func TestActive_GapEqualToBreakLengthResets(t *testing.T) {
	r := newRig(t, breakConfig())
	r.activeMinutes(0, 4)
	r.active(atMin(5)) // gap 2 < 3: kept
	if r.state.WorkCount != 5 {
		t.Fatalf("short gap: count = %d, want 5", r.state.WorkCount)
	}
	r.active(atMin(8)) // gap 3 == Break length: reset, then this minute counts
	if r.state.WorkCount != 1 {
		t.Fatalf("equal gap: count = %d, want 1", r.state.WorkCount)
	}
}

func TestTick_IdleGapResetsCount(t *testing.T) {
	r := newRig(t, breakConfig())
	r.activeMinutes(0, 3)
	r.tick(atMin(4))
	if r.state.WorkCount != 3 {
		t.Fatalf("count = %d, want 3", r.state.WorkCount)
	}
	r.tick(atMin(5))
	if r.state.WorkCount != 0 {
		t.Fatalf("count = %d, want 0 after a Break length of idle time", r.state.WorkCount)
	}
}

func TestActive_NothingCountsWhenOff(t *testing.T) {
	cases := map[string]func(*config.MentalHealthConfig){
		"master off": func(c *config.MentalHealthConfig) { c.Enabled = false },
		"break off":  func(c *config.MentalHealthConfig) { c.Break.Enabled = false },
	}
	for name, off := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := breakConfig()
			off(&cfg)
			r := newRig(t, cfg)
			r.activeMinutes(0, 5)
			if r.state.WorkCount != 0 || r.last.View.Counting {
				t.Fatalf("count = %d counting = %v, want 0 and false", r.state.WorkCount, r.last.View.Counting)
			}
		})
	}
}

func TestBreak_StartsAtWorkTimeAndEndsAfterBreakLength(t *testing.T) {
	r := newRig(t, breakConfig())
	r.activeMinutes(0, 9)
	if r.last.View.Period != nil {
		t.Fatal("Break started early")
	}
	res := r.active(at(10, 9, 30))
	p := res.View.Period
	if p == nil || p.Kind != KindBreak {
		t.Fatalf("period = %+v, want a Break", p)
	}
	if !p.End.Equal(at(10, 12, 30)) {
		t.Fatalf("end = %v, want 10:12:30", p.End)
	}
	if got := recordKinds(res.Records); !slices.Equal(got, []RecordKind{RecordBreakStart}) || !res.Records[0].Time.Equal(at(10, 9, 30)) {
		t.Fatalf("records = %+v, want one break-start at the start", res.Records)
	}
	if res.View.Counting {
		t.Fatal("counting during a Break")
	}

	r.active(atMin(10)) // no minute counts during the Break
	if r.state.WorkCount != 10 {
		t.Fatalf("count moved during the Break: %d", r.state.WorkCount)
	}
	res = r.tick(at(10, 12, 29))
	if res.View.Period == nil {
		t.Fatal("Break ended early")
	}
	res = r.tick(at(10, 12, 30))
	if res.View.Period != nil || r.state.WorkCount != 0 {
		t.Fatalf("after the Break: period = %+v count = %d", res.View.Period, r.state.WorkCount)
	}
}

func TestBreak_BuiltinMessageIsSeededAndStored(t *testing.T) {
	cfg := breakConfig()
	cfg.Break.WorkMinutes = 3
	want := rand.New(rand.NewPCG(1, 2)).IntN(len(builtinBreakMessages))
	r := newRig(t, cfg)
	r.activeMinutes(0, 3)
	pair := builtinBreakMessages[want]
	if got := r.last.View.Period; got.Message != strings.ReplaceAll(pair.message, "{work}", "3") ||
		got.SubMessage != strings.ReplaceAll(pair.sub, "{work}", "3") {
		t.Fatalf("message = %q / %q, want pair %d", got.Message, got.SubMessage, want)
	}
	stored := r.state.Period.Message
	if res := r.tick(atMin(4)); res.View.Period.Message != stored {
		t.Fatal("message changed after the start")
	}
}

func TestBreak_BuiltinListFillsWorkTime(t *testing.T) {
	for i, pair := range builtinBreakMessages {
		if pair.message == "" || pair.sub == "" {
			t.Fatalf("pair %d is empty", i)
		}
	}
	if len(builtinBreakMessages) != 12 {
		t.Fatalf("%d built-in messages, want 12", len(builtinBreakMessages))
	}
}

func TestBreak_CustomMessageSplitsAtFirstSentenceEnd(t *testing.T) {
	cases := []struct{ text, msg, sub string }{
		{"Stop now. Go for a walk! Really.", "Stop now.", "Go for a walk! Really."},
		{"Is it time? Yes.", "Is it time?", "Yes."},
		{"Version 1.5 is fine! Rest.", "Version 1.5 is fine!", "Rest."},
		{"One sentence only.", "One sentence only.", ""},
	}
	for _, c := range cases {
		cfg := breakConfig()
		cfg.Break.WorkMinutes = 3
		cfg.Break.Message = c.text
		r := newRig(t, cfg)
		r.activeMinutes(0, 3)
		if got := r.last.View.Period; got.Message != c.msg || got.SubMessage != c.sub {
			t.Fatalf("%q split into %q / %q, want %q / %q", c.text, got.Message, got.SubMessage, c.msg, c.sub)
		}
	}
}

func TestBedtime_RangeCrossingMidnightAndWithinADay(t *testing.T) {
	cases := []struct {
		start, end string
		h, m       int
		want       bool
	}{
		{"22:00", "07:00", 23, 30, true},
		{"22:00", "07:00", 6, 0, true},
		{"22:00", "07:00", 12, 0, false},
		{"22:00", "07:00", 7, 0, false},
		{"22:00", "07:00", 22, 0, true},
		{"08:00", "17:00", 12, 0, true},
		{"08:00", "17:00", 20, 0, false},
	}
	for _, c := range cases {
		r := newRig(t, bedtimeConfig(c.start, c.end))
		res := r.tick(at(c.h, c.m, 0))
		if got := res.View.Period != nil && res.View.Period.Kind == KindBedtime; got != c.want {
			t.Errorf("%s-%s at %02d:%02d: bedtime = %v, want %v", c.start, c.end, c.h, c.m, got, c.want)
		}
	}
}

func TestBedtime_EndsAtEndAcrossMidnightAndResetsCount(t *testing.T) {
	r := newRig(t, bedtimeConfig("22:00", "07:00"))
	res := r.tick(at(23, 30, 0))
	wantEnd := time.Date(2026, 11, 17, 7, 0, 0, 0, testZone)
	if !res.View.Period.End.Equal(wantEnd) {
		t.Fatalf("end = %v, want %v", res.View.Period.End, wantEnd)
	}
	if got := recordKinds(res.Records); len(got) != 0 {
		t.Fatalf("bedtime start wrote records: %v", got)
	}
	r.state.WorkCount = 4
	if res := r.tick(wantEnd); res.View.Period != nil || r.state.WorkCount != 0 {
		t.Fatalf("after End: period = %+v count = %d", res.View.Period, r.state.WorkCount)
	}
}

func TestBedtime_UsesTheLocationOfTheGivenTime(t *testing.T) {
	r := newRig(t, bedtimeConfig("22:00", "07:00"))
	instant := time.Date(2026, 11, 16, 23, 0, 0, 0, testZone)
	if res := r.tick(instant.UTC()); res.View.Period != nil {
		t.Fatal("21:00 UTC is outside the range")
	}
	if res := r.tick(instant); res.View.Period == nil {
		t.Fatal("23:00 in the test zone is inside the range")
	}
}

func TestBedtime_MessageRendersEnd(t *testing.T) {
	r := newRig(t, bedtimeConfig("22:00", "07:00"))
	p := r.tick(at(23, 0, 0)).View.Period
	if p.Message != "That's enough AI for today." || p.SubMessage != "Go to sleep and come back tomorrow at 07:00." {
		t.Fatalf("message = %q / %q", p.Message, p.SubMessage)
	}
}

func TestBedtime_TurnedOnInsideRangeStartsAtOnce(t *testing.T) {
	cfg := bedtimeConfig("22:00", "07:00")
	cfg.Bedtime.Enabled = false
	r := newRig(t, cfg)
	if r.tick(at(23, 0, 0)).View.Period != nil {
		t.Fatal("Bedtime off still started")
	}
	on := cfg
	on.Bedtime.Enabled = true
	if res := r.settings(at(23, 0, 1), on); res.View.Period == nil || res.View.Period.Kind != KindBedtime {
		t.Fatalf("period = %+v, want Bedtime with no grace", res.View.Period)
	}
}

func TestBedtime_NotStartedByBreakOnlyConfig(t *testing.T) {
	r := newRig(t, breakConfig())
	if r.tick(at(23, 0, 0)).View.Period != nil {
		t.Fatal("Bedtime started with Bedtime off")
	}
}

func TestOverlap_BedtimeDropsRunningBreak(t *testing.T) {
	cfg := bedtimeConfig("22:00", "07:00")
	cfg.Break = config.MentalHealthBreak{Enabled: true, WorkMinutes: 3, BreakMinutes: 30}
	r := newRig(t, cfg)
	for i := range 3 {
		r.active(at(21, 40+i, 0))
	}
	if r.state.Period == nil || r.state.Period.Kind != KindBreak {
		t.Fatalf("period = %+v, want a Break", r.state.Period)
	}
	started := r.state.Period.Start
	res := r.tick(at(22, 0, 0))
	if res.View.Period.Kind != KindBedtime || r.state.WorkCount != 0 {
		t.Fatalf("period = %+v count = %d", res.View.Period, r.state.WorkCount)
	}
	if len(res.Records) != 1 || res.Records[0].Kind != RecordBreakDropped || !res.Records[0].Started.Equal(started) {
		t.Fatalf("records = %+v, want one drop naming the start", res.Records)
	}
}

func TestOverlap_DueBreakAtBedtimeStartNeverStarts(t *testing.T) {
	cfg := bedtimeConfig("22:00", "07:00")
	cfg.Break = config.MentalHealthBreak{Enabled: true, WorkMinutes: 3, BreakMinutes: 30}
	r := newRig(t, cfg)
	r.active(at(21, 58, 0))
	r.active(at(21, 59, 0))
	res := r.active(at(22, 0, 0)) // would be the third minute
	if res.View.Period.Kind != KindBedtime || r.state.WorkCount != 0 || len(res.Records) != 0 {
		t.Fatalf("period = %+v count = %d records = %+v", res.View.Period, r.state.WorkCount, res.Records)
	}
}

func TestWarning_BedtimeFromTenMinutesBefore(t *testing.T) {
	r := newRig(t, bedtimeConfig("22:00", "07:00"))
	if r.tick(at(21, 49, 59)).View.Warning != nil {
		t.Fatal("warning too early")
	}
	w := r.tick(at(21, 50, 0)).View.Warning
	if w == nil || w.Kind != KindBedtime || !w.Start.Equal(at(22, 0, 0)) {
		t.Fatalf("warning = %+v, want Bedtime at 22:00", w)
	}
	if res := r.tick(at(22, 0, 0)); res.View.Warning != nil {
		t.Fatalf("warning after the start: %+v", res.View.Warning)
	}
}

func TestWarning_BreakAtTwoBelowWorkTimeAndMoves(t *testing.T) {
	r := newRig(t, breakConfig())
	r.activeMinutes(0, 7)
	if r.last.View.Warning != nil {
		t.Fatal("warning at 7 of 10")
	}
	r.active(atMin(7))
	w := r.last.View.Warning
	if w == nil || w.Kind != KindBreak || !w.Start.Equal(atMin(9)) || w.BreakMinutes != 3 {
		t.Fatalf("warning = %+v, want Break at 10:09 for 3 minutes", w)
	}
	// An idle minute moves the predicted start; it stays one warning.
	w = r.tick(atMin(9)).View.Warning
	if w == nil || !w.Start.Equal(atMin(10)) {
		t.Fatalf("after an idle minute: %+v, want start 10:10", w)
	}
	r.active(atMin(9))
	if w := r.last.View.Warning; w == nil || !w.Start.Equal(atMin(10)) {
		t.Fatalf("after the next heartbeat: %+v", w)
	}
	// The count resets after a Break length of idle time: the warning ends.
	if r.tick(atMin(12)).View.Warning != nil {
		t.Fatal("warning survived the reset")
	}
}

func TestWarning_BreakNotRaisedWhenBedtimeStartsFirst(t *testing.T) {
	cfg := bedtimeConfig("22:00", "07:00")
	cfg.Break = config.MentalHealthBreak{Enabled: true, WorkMinutes: 10, BreakMinutes: 3}
	r := newRig(t, cfg)
	// Eight minutes counted: predicted start 21:58+2 = 22:00, at the Bedtime start.
	for i := range 8 {
		r.active(at(21, 51+i, 0))
	}
	if w := r.last.View.Warning; w == nil || w.Kind != KindBedtime {
		t.Fatalf("warning = %+v, want only the Bedtime warning", w)
	}
}

func TestWarning_AtMostOneAndBreakWinsWhenEarlier(t *testing.T) {
	cfg := bedtimeConfig("22:00", "07:00")
	cfg.Break = config.MentalHealthBreak{Enabled: true, WorkMinutes: 10, BreakMinutes: 3}
	r := newRig(t, cfg)
	for i := range 8 {
		r.active(at(21, 50+i, 0))
	}
	if w := r.last.View.Warning; w == nil || w.Kind != KindBreak || !w.Start.Equal(at(21, 59, 0)) {
		t.Fatalf("warning = %+v, want the Break warning at 21:59", w)
	}
}

const goodSentence = "I'm tired and I'm addicted to AI"

func TestUnlock_SentenceVariants(t *testing.T) {
	good := []string{
		goodSentence,
		"i'm tired and i'm addicted to ai",
		"  I’m  very tired  and I'm addicted to AI ",
		"I’m a b c and I’m ADDICTED to ai",
	}
	bad := []string{"", "hello", "I'm and I'm addicted to AI", "I am tired and I am addicted to AI", "I'm tired and I'm addicted to AI!"}
	for _, s := range good {
		r := newRig(t, bedtimeConfig("22:00", "07:00"))
		r.tick(at(23, 0, 0))
		if res := r.unlock(at(23, 1, 0), s); res.Refusal != "" {
			t.Errorf("%q refused: %s", s, res.Refusal)
		}
	}
	for _, s := range bad {
		r := newRig(t, bedtimeConfig("22:00", "07:00"))
		r.tick(at(23, 0, 0))
		before := r.state
		res := r.unlock(at(23, 1, 0), s)
		if res.Refusal != ReasonSentenceMismatch || res.State != before || len(res.Records) != 0 || res.View.Period == nil {
			t.Errorf("%q: refusal = %q, changed = %v", s, res.Refusal, res.State != before)
		}
	}
}

func TestUnlock_NoRestPeriodIsRefused(t *testing.T) {
	r := newRig(t, breakConfig())
	if res := r.unlock(atMin(0), goodSentence); res.Refusal != ReasonNoRestPeriod {
		t.Fatalf("refusal = %q", res.Refusal)
	}
}

func TestUnlock_BreakEndsAtOnceQuietUntilOriginalEndAndRestartsCount(t *testing.T) {
	r := newRig(t, breakConfig())
	r.activeMinutes(0, 10)
	end := r.state.Period.End
	res := r.unlock(at(10, 10, 0), goodSentence)
	if res.Refusal != "" || res.View.Period != nil || r.state.WorkCount != 0 {
		t.Fatalf("refusal = %q period = %+v count = %d", res.Refusal, res.View.Period, r.state.WorkCount)
	}
	if n := len(res.Records); n != 1 || res.Records[0].Kind != RecordUnlock || res.Records[0].Rest != KindBreak {
		t.Fatalf("records = %+v", res.Records)
	}
	if !r.state.QuietUntil.Equal(end) || !res.View.Quiet() {
		t.Fatalf("quiet until %v quiet = %v, want until %v", r.state.QuietUntil, res.View.Quiet(), end)
	}
	r.active(at(10, 11, 0)) // counting resumes at once
	if r.state.WorkCount != 1 {
		t.Fatalf("count = %d, want 1", r.state.WorkCount)
	}
	if r.tick(end).View.Quiet() {
		t.Fatal("still quiet at the original end")
	}
}

func TestUnlock_BedtimeStaysOffUntilEndEvenIfRangeEdited(t *testing.T) {
	cfg := bedtimeConfig("22:00", "07:00")
	cfg.Break = config.MentalHealthBreak{Enabled: true, WorkMinutes: 10, BreakMinutes: 3}
	r := newRig(t, cfg)
	r.tick(at(23, 0, 0))
	r.unlock(at(23, 5, 0), goodSentence)
	if r.state.UnlockedKind != KindBedtime {
		t.Fatal("unlock kind not kept")
	}

	r.active(at(23, 6, 0))
	if r.state.WorkCount != 0 || r.last.View.Counting {
		t.Fatal("a minute counted during an unlocked Bedtime")
	}
	edited := cfg
	edited.Bedtime.Start = "23:00"
	if res := r.settings(at(23, 10, 0), edited); res.View.Period != nil {
		t.Fatal("editing the range restarted an unlocked Bedtime")
	}
	end := time.Date(2026, 11, 17, 7, 0, 0, 0, testZone)
	if res := r.tick(end.Add(-time.Second)); res.View.Period != nil || !res.View.Quiet() {
		t.Fatalf("before End: period = %+v quiet = %v", res.View.Period, res.View.Quiet())
	}
	r.tick(end)
	r.active(end.Add(time.Minute))
	if r.state.WorkCount != 1 {
		t.Fatalf("count = %d, want 1 after the unlocked Bedtime ended", r.state.WorkCount)
	}
}

func TestSettings_EndRestPeriodAtOnce(t *testing.T) {
	cases := map[string]struct {
		cfg    func() config.MentalHealthConfig
		change func(*config.MentalHealthConfig)
		start  func(*rig)
	}{
		"master off ends a Break": {
			cfg: breakConfig, change: func(c *config.MentalHealthConfig) { c.Enabled = false },
			start: func(r *rig) { r.activeMinutes(0, 10) },
		},
		"break off ends a Break": {
			cfg: breakConfig, change: func(c *config.MentalHealthConfig) { c.Break.Enabled = false },
			start: func(r *rig) { r.activeMinutes(0, 10) },
		},
		"master off ends Bedtime": {
			cfg:    func() config.MentalHealthConfig { return bedtimeConfig("22:00", "07:00") },
			change: func(c *config.MentalHealthConfig) { c.Enabled = false },
			start:  func(r *rig) { r.tick(at(23, 0, 0)) },
		},
		"bedtime off ends Bedtime": {
			cfg:    func() config.MentalHealthConfig { return bedtimeConfig("22:00", "07:00") },
			change: func(c *config.MentalHealthConfig) { c.Bedtime.Enabled = false },
			start:  func(r *rig) { r.tick(at(23, 0, 0)) },
		},
		"range that no longer holds the time ends Bedtime": {
			cfg:    func() config.MentalHealthConfig { return bedtimeConfig("22:00", "07:00") },
			change: func(c *config.MentalHealthConfig) { c.Bedtime.Start, c.Bedtime.End = "08:00", "17:00" },
			start:  func(r *rig) { r.tick(at(23, 0, 0)) },
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, c.cfg())
			c.start(r)
			if r.state.Period == nil {
				t.Fatal("no Rest period to end")
			}
			next := c.cfg()
			c.change(&next)
			now := r.state.Period.Start.Add(time.Second)
			res := r.settings(now, next)
			if res.View.Period != nil || res.View.Quiet() {
				t.Fatalf("period = %+v quiet = %v", res.View.Period, res.View.Quiet())
			}
		})
	}
}

func TestSettings_BreakOffOrMasterOffClearsCountButBedtimeOffDoesNot(t *testing.T) {
	cfg := breakConfig()
	cfg.Bedtime.Enabled = true
	cfg.Bedtime.Start, cfg.Bedtime.End = "22:00", "07:00"
	for name, change := range map[string]func(*config.MentalHealthConfig){
		"break off":  func(c *config.MentalHealthConfig) { c.Break.Enabled = false },
		"master off": func(c *config.MentalHealthConfig) { c.Enabled = false },
	} {
		r := newRig(t, cfg)
		r.activeMinutes(0, 2)
		next := cfg
		change(&next)
		r.settings(atMin(2), next)
		if r.state.WorkCount != 0 {
			t.Errorf("%s: count = %d, want 0", name, r.state.WorkCount)
		}
		r.settings(atMin(2), cfg)
		r.active(atMin(2))
		if r.state.WorkCount != 1 {
			t.Errorf("%s: count after turning back on = %d, want a fresh 1", name, r.state.WorkCount)
		}
	}
	r := newRig(t, cfg)
	r.activeMinutes(0, 2)
	next := cfg
	next.Bedtime.Enabled = false
	r.settings(atMin(2), next)
	if r.state.WorkCount != 2 {
		t.Errorf("bedtime off: count = %d, want 2", r.state.WorkCount)
	}
}

func TestSettings_WorkTimeChangeAppliesAtOnce(t *testing.T) {
	r := newRig(t, breakConfig())
	r.activeMinutes(0, 5)
	shorter := breakConfig()
	shorter.Break.WorkMinutes = 6
	r.settings(atMin(5), shorter)
	if res := r.active(atMin(5)); res.View.Period == nil {
		t.Fatal("Break did not start at the new Work time")
	}
}

func TestQuiet_OnlyWhileFeatureOnAndRestPeriodRunsOrQuietUntilIsFuture(t *testing.T) {
	cfg := breakConfig()
	now := atMin(0)
	until := now.Add(time.Hour)
	cases := []struct {
		name  string
		state State
		on    bool
		want  bool
	}{
		{"nothing", State{}, true, false},
		{"period", State{Period: &Period{Kind: KindBreak}}, true, true},
		{"quiet until in the future", State{QuietUntil: until}, true, true},
		{"quiet until passed", State{QuietUntil: now}, true, false},
		{"feature off", State{Period: &Period{Kind: KindBreak}, QuietUntil: until}, false, false},
	}
	for _, c := range cases {
		cfg.Enabled = c.on
		if got := c.state.Quiet(cfg, now); got != c.want {
			t.Errorf("%s: quiet = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestStep_FreshStateStaysEmptyWhileOff(t *testing.T) {
	r := newRig(t, config.DefaultMentalHealthConfig())
	for i := range 120 {
		r.active(atMin(0).Add(time.Duration(i) * time.Minute))
	}
	v := r.last.View
	if v.Period != nil || v.Warning != nil || v.Quiet() || v.Counting || r.state != (State{}) {
		t.Fatalf("view = %+v state = %+v", v, r.state)
	}
}
