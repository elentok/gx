package webflows

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/rest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

func (r *running) restStatus(t *testing.T) wire.RestStatus {
	t.Helper()
	var res struct {
		Status wire.RestStatus `json:"status"`
	}
	if err := json.Unmarshal([]byte(r.Gx(t, "rest", "--json")), &res); err != nil {
		t.Fatal(err)
	}
	return res.Status
}

func TestRestScenario_EveryShapeSeedsWhatItNames(t *testing.T) {
	t.Parallel()
	for _, shape := range spec.RestShapes {
		t.Run(shape, func(t *testing.T) {
			t.Parallel()
			r := startWith(t, "rest", nil, "--shape", shape)
			st := r.restStatus(t)
			switch shape {
			case "bedtime":
				if st.Period == nil || st.Period.Kind != wire.RestKindBedtime {
					t.Fatalf("period = %+v, want Bedtime", st.Period)
				}
				if st.Rule.BedtimeStart == "" || st.Rule.BedtimeEnd == "" {
					t.Errorf("rule = %+v", st.Rule)
				}
			case "break":
				p := st.Period
				if p == nil || p.Kind != wire.RestKindBreak {
					t.Fatalf("period = %+v, want Break", p)
				}
				if left := p.End.Sub(st.Now); left < 3*time.Minute || left > 5*time.Minute {
					t.Errorf("Break ends in %v, want about 4m37s", left)
				}
				if p.Message != spec.RestBreakMessage || p.SubMessage != spec.RestBreakSubMessage {
					t.Errorf("text = %q / %q", p.Message, p.SubMessage)
				}
				files, _ := filepath.Glob(filepath.Join(r.StateDir, rest.RecordsDirName, "*.jsonl"))
				var all string
				for _, f := range files {
					b, _ := os.ReadFile(f)
					all += string(b)
				}
				if !strings.Contains(all, `"kind":"break-start"`) {
					t.Errorf("no break-start record in %v: %q", files, all)
				}
			case "break-warning":
				if st.Period != nil || st.Warning == nil || st.Warning.Kind != wire.RestKindBreak {
					t.Errorf("period %+v warning %+v, want a Break warning only", st.Period, st.Warning)
				}
			case "bedtime-warning":
				if st.Period != nil || st.Warning == nil || st.Warning.Kind != wire.RestKindBedtime {
					t.Fatalf("period %+v warning %+v, want a Bedtime warning only", st.Period, st.Warning)
				}
				if d := st.Warning.Start.Sub(st.Now); d < 8*time.Minute || d > 11*time.Minute {
					t.Errorf("Bedtime starts in %v, want about 10m", d)
				}
			}
		})
	}
}
