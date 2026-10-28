package server_test

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

const (
	recoveryAddr   = "proj:epic-a/01"
	recoveryBranch = "ralph-loop/epic-a-item-01"
)

func gitIn(t *testing.T, repo string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// startRecoveryWeb serves a repair-paused ticket 01 over the web mux and returns
// the repo, the session cookie and the web base URL.
func startRecoveryWeb(t *testing.T) (h *servertest.Harness, repo, base string, cookie *http.Cookie) {
	t.Helper()
	store := t.TempDir()
	repo = testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	h = servertest.StartWithStore(t, store, func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	registerLaunch(h)
	gitIn(t, repo, "branch", "epic-a")
	gitIn(t, repo, "branch", recoveryBranch)
	wt := filepath.Join(t.TempDir(), "wt")
	gitIn(t, repo, "worktree", "add", wt, recoveryBranch)
	testutil.WriteFile(t, wt, "work.txt", "work")
	testutil.CommitAll(t, wt, "agent work")
	gitIn(t, repo, "worktree", "remove", "--force", wt)
	if err := os.MkdirAll(filepath.Join(repo, ".worktrees", "epic-a"), 0o755); err != nil {
		t.Fatal(err)
	}
	hs, err := h.Client.Handshake(t.Context())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake = %+v, %v", hs, err)
	}
	if err := h.Server.PauseAs(recoveryAddr, events.IterationError, "boom"); err != nil {
		t.Fatal(err)
	}
	return h, repo, hs.WebURL, login(t, h.StateDir, hs.WebURL)
}

func detailState(t *testing.T, h *servertest.Harness, base string) string {
	t.Helper()
	code, body := webDo(t, h, http.MethodGet, base+"/v1/tickets/detail?address="+recoveryAddr)
	if code != http.StatusOK {
		t.Fatalf("detail = %d %s", code, body)
	}
	var d struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatal(err)
	}
	return d.State
}

func TestWebDetail_RepairPauseCarriesAStateToken(t *testing.T) {
	t.Parallel()
	h, _, base, _ := startRecoveryWeb(t)
	if state := detailState(t, h, base); state == "" {
		t.Error("repair pause detail has no state token")
	}
}

func TestWebRecoveryVerbs_OldTokenIsStaleCurrentRuns(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"/v1/tickets/retry", "/v1/tickets/merge", "/v1/tickets/start-over"} {
		t.Run(path, func(t *testing.T) {
			h, _, base, cookie := startRecoveryWeb(t)
			state := detailState(t, h, base)
			post := func(token string) string {
				body := `{"address":"` + recoveryAddr + `","state":"` + token + `","reason":"stuck"}`
				code, out := webPostJSON(t, base+path, body, cookie, "", jsonPost(base))
				if code != http.StatusOK {
					t.Fatalf("POST %s = %d %s", path, code, out)
				}
				return out
			}
			if got := reasonOf(t, post("99:repair")); got != server.ReasonStale {
				t.Errorf("old token: reason = %q, want %q", got, server.ReasonStale)
			}
			if got := reasonOf(t, post(state)); got == server.ReasonStale {
				t.Errorf("current token was refused as stale")
			}
		})
	}
}

func TestWebStartOver_DeleteFlagKeepsTheBranch(t *testing.T) {
	t.Parallel()
	h, repo, base, cookie := startRecoveryWeb(t)
	body := `{"address":"` + recoveryAddr + `","state":"` + detailState(t, h, base) + `","reason":"stuck","delete_branch":true}`
	code, out := webPostJSON(t, base+"/v1/tickets/start-over", body, cookie, "", jsonPost(base))
	if code != http.StatusOK || strings.Contains(out, `"refused"`) {
		t.Fatalf("start-over = %d %s", code, out)
	}
	// Start over sets the branch aside; a delete would leave no ref behind.
	if refs := gitIn(t, repo, "for-each-ref", "--format=%(refname)"); !strings.Contains(refs, "attic/epic-a/01") {
		t.Errorf("no ref for the iteration branch is left: %q", refs)
	}
}
