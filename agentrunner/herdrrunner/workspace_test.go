package herdrrunner_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/testutil/herdrfake"
)

func TestStart_ParallelStartsCreateTheEpicWorkspaceOnce(t *testing.T) {
	state := herdrfake.NewState(t)
	_, fakeEnv := herdrfake.StartAgentHostEnv(t, state)
	r := newTestRunner(fakeEnv)
	r.BackgroundTasks = nil

	const n = 6
	var wg sync.WaitGroup
	errs := make(chan error, n+1)
	start := func(epic string, i int) {
		defer wg.Done()
		_, err := r.Start(agentrunner.StartOptions{
			Label: fmt.Sprintf("%s-%02d", epic, i), Epic: epic, Cwd: t.TempDir(), Kind: "claude",
		})
		errs <- err
	}
	for i := range n {
		wg.Add(1)
		go start("e", i)
	}
	wg.Add(1)
	go start("other", 0)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	perLabel := map[string]int{}
	for _, w := range state.Workspaces {
		perLabel[w.Label]++
	}
	if perLabel["e"] != 1 || perLabel["other"] != 1 || len(state.Workspaces) != 2 {
		t.Fatalf("workspaces by label = %v, want one each for e and other", perLabel)
	}
	tabs := map[string]int{}
	for _, tab := range state.Tabs {
		tabs[state.Workspaces[tab.WorkspaceID].Label]++
	}
	if tabs["e"] != n || tabs["other"] != 1 {
		t.Fatalf("tabs by workspace = %v, want %d in e and 1 in other", tabs, n)
	}
}
