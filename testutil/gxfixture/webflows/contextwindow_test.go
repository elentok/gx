package webflows

import (
	"encoding/json"
	"testing"

	"github.com/elentok/gx/server/wire"
)

func TestContextWindowFixture_SessionsShowTheirContextCount(t *testing.T) {
	r := start(t, "context-window")
	defer r.Stop(t)

	for _, key := range []string{"green", "yellow", "orange", "red", "compact"} {
		if r.Transcripts[key] == "" {
			t.Errorf("no %s transcript in the output: %v", key, r.Transcripts)
		}
	}

	var list []wire.SessionInfo
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	counts := map[int]bool{}
	var queue, exited, withoutCount int
	for _, si := range list {
		if si.Queue {
			queue++
		}
		if si.Status.State == wire.SessionExited {
			exited++
		}
		if si.ContextTokens == nil {
			withoutCount++
			continue
		}
		counts[*si.ContextTokens] = true
	}
	for _, want := range []int{40_000, 90_000, 128_000, 170_000} {
		if !counts[want] {
			t.Errorf("no session shows a context count of %d: %v", want, counts)
		}
	}
	if queue != 1 || exited < 1 || withoutCount == 0 {
		t.Errorf("want one Queue agent, a finished session and some without a count: queue %d, exited %d, no count %d of %d", queue, exited, withoutCount, len(list))
	}
}
