package evolve

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
)

func appendLaunch(t *testing.T, f *evolveFixture, ticket string, total, deps time.Duration) {
	t.Helper()
	ev := runlog.Event{Type: string(events.LaunchTimed), Ticket: ticket, Total: total, DepsStep: deps, PromptStep: total - deps}
	if err := runlog.AppendEvent(filepath.Dir(f.epic), "epic", ev); err != nil {
		t.Fatal(err)
	}
}

func TestEvolveInputs_LaunchRows(t *testing.T) {
	t.Parallel()
	f := inputsFixture(t)
	appendLaunch(t, f, "01", 40*time.Second, 30*time.Second)
	appendLaunch(t, f, "02", 20*time.Second, 5*time.Second)
	appendLaunch(t, f, "04", 2*time.Second, time.Second)
	appendLaunch(t, f, "07", 3*time.Second, time.Second)
	base := inputsTable(t, inputsFixture(t))

	table := inputsTable(t, f)
	if len(table.Launches) != 2 {
		t.Fatalf("launches = %+v, want rows for 01 and 02 and no median (median is under the limit)", table.Launches)
	}
	if got := table.Launches[0]; got.Ticket != "01" || got.TotalMS != 40_000 || got.SlowestStep != "deps" {
		t.Errorf("first row = %+v, want 01, 40000ms, deps", got)
	}
	for _, r := range table.Rows {
		if r.ID == "02" && (!slices.Contains(r.Flags, flagSlowLaunch) || r.Severity != severityRest) {
			t.Errorf("row 02 = flags %v severity %d, want slow-launch at severity rest", r.Flags, r.Severity)
		}
	}
	if len(base.Launches) != 0 {
		t.Errorf("no launch events gave rows %+v", base.Launches)
	}
}

func TestEvolveInputs_SlowMedianLaunchRowPrintsNextToChecks(t *testing.T) {
	t.Parallel()
	f := inputsFixture(t)
	appendLaunch(t, f, "01", 40*time.Second, 30*time.Second)
	appendLaunch(t, f, "02", 30*time.Second, 5*time.Second)
	var out bytes.Buffer
	if err := runEvolveInputs(f.env, &out, "proj:epic", false); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"### Launch rows", "launch: 01 40s, slowest step deps", "launch: median 35s", "slow-launch"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
}
