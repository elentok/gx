package rest_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/rest"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

var zone = time.FixedZone("test", 2*60*60)

func start() time.Time { return time.Date(2026, 11, 16, 10, 0, 0, 0, zone) }

func breakCfg() config.MentalHealthConfig {
	cfg := config.DefaultMentalHealthConfig()
	cfg.Enabled = true
	cfg.Break = config.MentalHealthBreak{Enabled: true, WorkMinutes: 3, BreakMinutes: 2}
	return cfg
}

// runnerRig is a runner over a temp state dir with a fake clock and live config.
type runnerRig struct {
	dir    string
	events *broker.Broker
	now    time.Time
	cfg    config.MentalHealthConfig
	*rest.Runner
}

func openRig(t *testing.T, dir string, now time.Time, cfg config.MentalHealthConfig) *runnerRig {
	t.Helper()
	g := &runnerRig{dir: dir, events: broker.New(64), now: now, cfg: cfg}
	r, err := rest.Open(dir, g.events, func() config.MentalHealthConfig { return g.cfg }, func() time.Time { return g.now })
	if err != nil {
		t.Fatal(err)
	}
	g.Runner = r
	t.Cleanup(r.Close)
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	return g
}

// workToBreak counts active minutes until the Break starts.
func (g *runnerRig) workToBreak(t *testing.T) {
	t.Helper()
	for range g.cfg.Break.WorkMinutes {
		if _, err := g.Active(); err != nil {
			t.Fatal(err)
		}
		g.now = g.now.Add(time.Minute)
	}
}

// Seam: the runner counts through the live settings, and two callers in one
// minute count once.
func TestRunner_CountsOncePerMinuteThroughLiveSettings(t *testing.T) {
	t.Parallel()
	g := openRig(t, t.TempDir(), start(), breakCfg())
	for range 2 {
		if _, err := g.Active(); err != nil {
			t.Fatal(err)
		}
	}
	res, _ := g.Active()
	if res.State.WorkCount != 1 {
		t.Fatalf("WorkCount = %d, want 1", res.State.WorkCount)
	}
	g.cfg.Enabled = false
	g.now = g.now.Add(time.Minute)
	if res, _ = g.Active(); res.State.WorkCount != 0 {
		t.Errorf("WorkCount = %d after the master toggle went off, want 0", res.State.WorkCount)
	}
}

// Seam: a Break survives a restart with its message and quiet-until time.
// Seam: Quiet answers from the recomputed view, so a Break that has passed its
// end stops being quiet before any timer tick saves the new state.
func TestRunner_BreakTextOptionSetsTheBreakLines(t *testing.T) {
	t.Parallel()
	now, cfg := start(), breakCfg()
	r, err := rest.Open(t.TempDir(), broker.New(0), func() config.MentalHealthConfig { return cfg },
		func() time.Time { return now }, rest.WithBreakText("Stand up.", "Drink water."))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	for range cfg.Break.WorkMinutes {
		if _, err := r.Active(); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Minute)
	}
	if p := r.View().Period; p == nil || p.Message != "Stand up." || p.SubMessage != "Drink water." {
		t.Fatalf("Break text = %+v, want the two option lines", p)
	}
}

func TestRunner_QuietAgreesWithViewAfterBreakEndsWithoutTick(t *testing.T) {
	t.Parallel()
	g := openRig(t, t.TempDir(), start(), breakCfg())
	g.workToBreak(t)
	if !g.Quiet() {
		t.Fatal("Quiet() = false during the Break, want true")
	}
	g.now = g.now.Add(time.Duration(g.cfg.Break.BreakMinutes+1) * time.Minute)
	if g.Quiet() || g.Quiet() != g.View().Quiet() {
		t.Errorf("after the Break's end: Quiet() = %v, View().Quiet() = %v, want both false", g.Quiet(), g.View().Quiet())
	}
}

func TestRunner_RestartKeepsBreakAndQuietUntil(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	g := openRig(t, dir, start(), breakCfg())
	g.workToBreak(t)
	p := g.View().Period
	if p == nil || p.Kind != rest.KindBreak {
		t.Fatalf("period = %+v, want a Break", p)
	}
	g.Close()

	g2 := openRig(t, dir, g.now, g.cfg)
	got := g2.View().Period
	if got == nil || got.Message != p.Message || got.SubMessage != p.SubMessage || !got.End.Equal(p.End) {
		t.Fatalf("period after restart = %+v, want %+v", got, p)
	}
	if _, err := os.Stat(filepath.Join(dir, "rest.json.tmp")); !os.IsNotExist(err) {
		t.Errorf("temp file left behind: %v", err)
	}

	res, err := g2.Unlock("I'm Dana and I'm addicted to AI")
	if err != nil || res.Refusal != "" {
		t.Fatalf("unlock = %+v, %v", res, err)
	}
	g2.Close()
	g3 := openRig(t, dir, g.now, g.cfg)
	if g3.View().Period != nil || !g3.View().Quiet() {
		t.Errorf("after an unlock and a restart: period %+v quiet %v, want none and quiet", g3.View().Period, g3.View().Quiet())
	}
}

// Seam: a wrong sentence comes back as the core's refusal, not an error.
func TestRunner_UnlockRefusal(t *testing.T) {
	t.Parallel()
	g := openRig(t, t.TempDir(), start(), breakCfg())
	g.workToBreak(t)
	res, err := g.Unlock("let me in")
	if err != nil || res.Refusal != wire.ReasonRestSentenceMismatch {
		t.Errorf("unlock = %q, %v, want %q", res.Refusal, err, wire.ReasonRestSentenceMismatch)
	}
}

// Seam: a rest.json written in the old format loads as the same period.
func TestRunner_LoadsOldFormatFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	old := `{"work_count":2,"period":{"kind":"break","start":"2026-11-16T10:03:00+02:00","end":"2026-11-16T10:05:00+02:00","message":"Stretch","sub_message":"Back in 2"}}`
	if err := os.WriteFile(filepath.Join(dir, "rest.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	g := openRig(t, dir, start().Add(4*time.Minute), breakCfg())
	p := g.View().Period
	wantEnd := time.Date(2026, 11, 16, 10, 5, 0, 0, zone)
	if p == nil || p.Kind != rest.KindBreak || p.Message != "Stretch" || p.SubMessage != "Back in 2" || !p.End.Equal(wantEnd) {
		t.Errorf("period = %+v, want a Break %q/%q ending %v", p, "Stretch", "Back in 2", wantEnd)
	}
}

// Seam: each state change publishes one event, and a no-change call none.
func TestRunner_PublishesOneEventPerChange(t *testing.T) {
	t.Parallel()
	g := openRig(t, t.TempDir(), start(), breakCfg())
	evs, cancel, err := g.events.Subscribe(g.events.CurrentSeq())
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if _, err := g.Active(); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Active(); err != nil { // same minute: nothing changes
		t.Fatal(err)
	}
	if ev := <-evs; ev.Type != wire.EventRestChanged {
		t.Fatalf("event = %q, want %q", ev.Type, wire.EventRestChanged)
	}
	select {
	case ev := <-evs:
		t.Errorf("second heartbeat in the same minute published %q", ev.Type)
	default:
	}
}

// Seam: Break starts, dropped Breaks and unlocks are recorded; Bedtime
// starting over a Break drops it.
func TestRunner_RecordsBreakDropAndUnlock(t *testing.T) {
	t.Parallel()
	cfg := breakCfg()
	cfg.Break.BreakMinutes = 30
	cfg.Bedtime = config.MentalHealthBedtime{Enabled: true, Start: "10:10", End: "11:00"}
	g := openRig(t, t.TempDir(), start(), cfg)
	g.workToBreak(t)
	g.now = g.now.Add(12 * time.Minute)
	if _, err := g.Tick(); err != nil {
		t.Fatal(err)
	}
	if res, err := g.Unlock("I'm Dana and I'm addicted to AI"); err != nil || res.Refusal != "" {
		t.Fatalf("unlock = %+v, %v", res, err)
	}
	recs, err := g.Records().Range(start(), g.now)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []rest.RecordKind
	for _, r := range recs {
		kinds = append(kinds, r.Kind)
	}
	want := []rest.RecordKind{rest.RecordBreakStart, rest.RecordBreakDropped, rest.RecordUnlock}
	if len(kinds) != len(want) || kinds[0] != want[0] || kinds[1] != want[1] || kinds[2] != want[2] {
		t.Errorf("records = %v, want %v", kinds, want)
	}
}

// Seam: the store reads by day and prunes past 90 days on write.
func TestRecords_RangeAndPrune(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	now := start()
	s, err := rest.OpenRecords(dir, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	old := now.AddDate(0, 0, -rest.RecordsRetentionDays-1)
	// Written straight to the old day: the prune runs on the next write.
	if err := s.Append(rest.Record{Kind: rest.RecordBreakStart, Time: old}); err != nil {
		t.Fatal(err)
	}
	yesterday := now.AddDate(0, 0, -1)
	if err := s.Append(
		rest.Record{Kind: rest.RecordUnlock, Time: now, Rest: rest.KindBreak},
		rest.Record{Kind: rest.RecordBreakStart, Time: yesterday},
	); err != nil {
		t.Fatal(err)
	}
	got, err := s.Range(old, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Kind != rest.RecordBreakStart || got[1].Kind != rest.RecordUnlock || got[1].Rest != rest.KindBreak {
		t.Fatalf("records = %+v, want yesterday's start then today's unlock (old one pruned)", got)
	}
	one, err := s.Range(now, now)
	if err != nil || len(one) != 1 || one[0].Kind != rest.RecordUnlock {
		t.Errorf("today = %+v, %v", one, err)
	}
}

// Seam: the wake covers the end of a Break and the Bedtime warning and start.
func TestNextWake(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultMentalHealthConfig()
	cfg.Enabled = true
	cfg.Bedtime.Enabled = true
	cfg.Bedtime.Start, cfg.Bedtime.End = "23:00", "07:00"
	now := start()
	got, ok := rest.NextWake(cfg, rest.State{}, now)
	if want := time.Date(2026, 11, 16, 22, 50, 0, 0, zone); !ok || !got.Equal(want) {
		t.Errorf("wake = %v, %v, want %v", got, ok, want)
	}
	end := now.Add(2 * time.Minute)
	st := rest.State{Period: &rest.Period{Kind: rest.KindBreak, Start: now, End: end}}
	if got, _ := rest.NextWake(cfg, st, now); !got.Equal(end) {
		t.Errorf("wake = %v, want the Break end %v", got, end)
	}
	if _, ok := rest.NextWake(config.DefaultMentalHealthConfig(), rest.State{}, now); ok {
		t.Error("a feature that is off has a wake")
	}
}

// Seam: with the feature off no Rest period shows however far the clock moves.
func TestServer_FeatureOffShowsNoRest(t *testing.T) {
	t.Parallel()
	clock := servertest.NewRestClock(start())
	h := servertest.Start(t, servertest.WithRestClock(clock))
	for range 48 {
		if _, err := h.Server.Rest().Active(); err != nil {
			t.Fatal(err)
		}
		h.AdvanceRest(clock, time.Hour)
	}
	if v := h.Server.Rest().View(); v.Period != nil || v.Warning != nil {
		t.Errorf("view = %+v, want no period and no warning", v)
	}
}

// Seam: a reload that turns the master toggle off ends a running Rest period.
func TestServer_ReloadEndsRunningBreak(t *testing.T) {
	t.Parallel()
	clock := servertest.NewRestClock(start())
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	h := servertest.Start(t, servertest.WithRestClock(clock), servertest.WithMentalHealth(breakCfg()), servertest.WithConfigFile(cfgPath))
	for range 3 {
		if _, err := h.Server.Rest().Active(); err != nil {
			t.Fatal(err)
		}
		h.AdvanceRest(clock, time.Minute)
	}
	if v := h.Server.Rest().View(); v.Period == nil {
		t.Fatal("no Break is running")
	}
	if err := os.WriteFile(cfgPath, []byte(`{"mental-health":{"enabled":false}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	servertest.Reload(t, h)
	if v := h.Server.Rest().View(); v.Period != nil {
		t.Errorf("period = %+v after the reload, want none", v.Period)
	}
}
