package serverapi

import (
	"bytes"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
)

func TestServerTips_HasJSONFlag(t *testing.T) {
	c, _, err := serverParent(Deps{}).Find([]string{"tips"})
	if err != nil || c.RunE == nil {
		t.Fatalf("server tips = %v, err %v; want a runnable verb", c, err)
	}
	if c.Flags().Lookup("json") == nil {
		t.Error("server tips has no --json")
	}
}

func TestServerTips_TextListsTheFolders(t *testing.T) {
	var buf bytes.Buffer
	tip := wire.Tip{ID: "spotlight", Title: "Keep worktrees out of Spotlight", Body: "steps", Folders: []string{"/repo/.worktrees"}}
	if err := printTips(&buf, []wire.Tip{tip}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{tip.Title, "/repo/.worktrees"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("output lacks %q: %s", want, buf.String())
		}
	}
}

func TestServerStats_HasJSONRangeAndProjectFlags(t *testing.T) {
	c, _, err := serverParent(Deps{}).Find([]string{"stats"})
	if err != nil || c.RunE == nil {
		t.Fatalf("server stats = %v, err %v; want a runnable verb", c, err)
	}
	for _, flag := range []string{"json", "range", "project"} {
		if c.Flags().Lookup(flag) == nil {
			t.Errorf("server stats has no --%s", flag)
		}
	}
}

func TestServerStats_JSONHoldsLimitBurnSeries(t *testing.T) {
	s := wire.Stats{LimitBurn: wire.StatsLimitBurns{
		ClaudeWeekly: wire.StatsLimitBurn{
			Days:     []wire.LimitBurnDay{{Date: "2026-11-01", Burn: 7}},
			Sum:      7,
			Forecast: &wire.LimitForecast{Status: wire.LimitForecastProjected, Pace: 7},
		},
	}}
	var buf bytes.Buffer
	if err := cmdbase.EncodeProvenance(&buf, s, cmdbase.ViaServer, cmdbase.CallerActor(func() (string, error) { return "", nil })); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"limit_burn"`, `"claude_weekly"`, `"codex_5h"`, `"reset_day"`, `"forecast"`} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("--json output lacks %s: %s", want, buf.String())
		}
	}
}

func TestServerStats_TextAndJSONCarryLaunch(t *testing.T) {
	med, share := int64(12500), 0.75
	s := wire.Stats{Launch: wire.StatsLaunch{Count: 4, MedianMs: &med, ShareUnderTgt: &share, TargetMs: 15000}}
	var text bytes.Buffer
	if err := printStats(&text, s); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"12.5s", "75%"} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("text lacks %s: %s", want, text.String())
		}
	}
	var buf bytes.Buffer
	if err := cmdbase.EncodeProvenance(&buf, s, cmdbase.ViaServer, cmdbase.CallerActor(func() (string, error) { return "", nil })); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"launch"`, `"median_ms"`, `"share_under_target"`, `"step_median_ms"`} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("--json output lacks %s: %s", want, buf.String())
		}
	}
}

func TestServerStats_JSONHoldsGrillingSeries(t *testing.T) {
	s := wire.Stats{Grilling: wire.StatsGrilling{
		Words:     []wire.StatsAmount{{Value: 12}},
		Questions: []wire.StatsAmount{{NoData: true}},
	}}
	var buf bytes.Buffer
	if err := cmdbase.EncodeProvenance(&buf, s, cmdbase.ViaServer, cmdbase.CallerActor(func() (string, error) { return "", nil })); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"grilling"`, `"words"`, `"questions"`} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("--json output lacks %s: %s", want, buf.String())
		}
	}
}
