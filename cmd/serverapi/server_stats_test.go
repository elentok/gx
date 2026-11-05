package serverapi

import (
	"bytes"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
)

func TestServerStats_HasJSONRangeAndProjectFlags(t *testing.T) {
	root := newRootCmd(deps{})
	c, _, err := root.Find([]string{"server", "stats"})
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
