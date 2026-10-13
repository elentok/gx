package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// trunkDeletedMidRun is a one-ticket epic whose trunk develop is deleted while
// the agent works. It returns develop's tip and a count of chat text sent.
func trunkDeletedMidRun(t *testing.T, autoMerge bool) (f *schedFixture, develop string, chatCount func(string) int) {
	t.Helper()
	var mu sync.Mutex
	var sent strings.Builder
	chat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		sent.Write(b)
		mu.Unlock()
	}))
	t.Cleanup(chat.Close)
	chatCount = func(s string) int {
		mu.Lock()
		defer mu.Unlock()
		return strings.Count(sent.String(), s)
	}
	chatCfg := func(c *server.Config) {
		c.Chat = ralphloop.ServerChatConfig{SlackWebhookURL: chat.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: testFlushInterval}
	}
	f = newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
		f.git(t, "branch", "-D", "develop")
	}, chatCfg)
	f.git(t, "branch", "develop")
	if autoMerge {
		servertest.SetProjectTrunk(t, f.store, "proj", f.repo, "develop")
	} else {
		servertest.SetProjectTrunkNoAutoMerge(t, f.store, "proj", f.repo, "develop")
	}
	return f, f.rev(t, "develop"), chatCount
}

func TestTrunkMissingAtLanding_TicketLandsOnItsRootAndTheRootParks(t *testing.T) {
	f, develop, chatCount := trunkDeletedMidRun(t, true)
	f.runEpic(t, server.EventRootParked)

	if body := readFile(t, f.ticketFile(t, "01")); !strings.Contains(body, "status: done") {
		t.Errorf("ticket did not land on its root's branch:\n%s", body)
	}
	if got := f.git(t, "show", schedEpic+":01.txt"); got != "01" {
		t.Errorf("%s:01.txt = %q, want the ticket's work", schedEpic, got)
	}
	if got := f.rev(t, "main"); got != develop {
		t.Errorf("main moved to %s", got)
	}
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline) && chatCount("trunk-missing: project") == 0; time.Sleep(50 * time.Millisecond) {
	}
	if chatCount("trunk-missing: project") == 0 {
		t.Errorf("no root park with reason trunk-missing reached chat")
	}
}

func TestTrunkMissingAtLanding_AutoMergeOffIsNotParked(t *testing.T) {
	f, _, _ := trunkDeletedMidRun(t, false)
	f.runEpic(t, server.EventRootCompleted)

	if body := readFile(t, f.ticketFile(t, "01")); !strings.Contains(body, "status: done") {
		t.Errorf("ticket not done:\n%s", body)
	}
}
