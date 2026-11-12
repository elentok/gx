package schedules

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	schedrec "github.com/elentok/gx/schedules"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

func TestMain(m *testing.M) { cmdtest.Main(m) }

// run starts a server holding one Schedule and runs `gx schedules <args>`.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "root", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	if _, err := schedrec.NewStore(store).Create("proj", schedrec.Schedule{Name: "Nightly", Prompt: "check the logs", Days: []time.Weekday{time.Monday}, Times: []string{"09:00"}}); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)
	var out bytes.Buffer
	env := cmdbase.Env{Stdout: &out, NewClient: func() (*apiclient.Client, error) { return h.Client, nil }}
	err := cmdtest.Run(t, env, args, New(Deps{Env: env}))
	return out.String(), err
}

func TestList_PrintsOneLinePerScheduleAndJSON(t *testing.T) {
	out, err := run(t, "schedules", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Nightly") || !strings.Contains(out, "off here") {
		t.Errorf("text = %q", out)
	}
	out, err = run(t, "schedules", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"schedules"`) || !strings.Contains(out, `"via"`) {
		t.Errorf("json = %q, want the result with provenance", out)
	}
}

func TestShow_PrintsPromptAndJSON(t *testing.T) {
	out, err := run(t, "schedules", "show", "proj:nightly")
	if err != nil || !strings.Contains(out, "check the logs") {
		t.Errorf("out = %q err = %v", out, err)
	}
	out, err = run(t, "schedules", "show", "proj:nightly", "--json")
	if err != nil || !strings.Contains(out, `"prompt"`) || !strings.Contains(out, `"via"`) {
		t.Errorf("json = %q err = %v", out, err)
	}
}

func TestShow_RefusalExitsNonZeroNamingTheReason(t *testing.T) {
	_, err := run(t, "schedules", "show", "proj:nope")
	if err == nil || !strings.Contains(err.Error(), "unknown-schedule") {
		t.Errorf("err = %v, want unknown-schedule", err)
	}
	out, err := run(t, "schedules", "show", "proj:nope", "--json")
	cmdtest.RequireRefusalExit(t, err)
	if !strings.Contains(out, `"unknown-schedule"`) {
		t.Errorf("json = %q", out)
	}
}
