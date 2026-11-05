package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	gxconfig "github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/testutil"
)

func TestExecute_Notify_Enable_ClearsTransportMuteAndReportsIt(t *testing.T) {
	repo := testutil.TempRepo(t)
	cmdtest.StoreProjectFor(t, repo)

	var stdout bytes.Buffer
	d := Deps{Env: cmdbase.Env{Stdout: &stdout, Stderr: bytes.NewBuffer(nil), Env: cmdtest.EnvFor(t)}}

	if err := run(t, d, "notify", "--disable", "telegram"); err != nil {
		t.Fatalf("execute notify --disable: %v", err)
	}
	stdout.Reset()

	if err := run(t, d, "notify", "--enable", "telegram"); err != nil {
		t.Fatalf("execute notify --enable: %v", err)
	}
	if !strings.Contains(stdout.String(), "telegram: active") {
		t.Fatalf("expected active report, got: %q", stdout.String())
	}

	var status bytes.Buffer
	d.Stdout = &status
	d.Getwd = func() (string, error) { return repo, nil }
	if err := run(t, d, "notify", "--status"); err != nil {
		t.Fatalf("execute notify --status: %v", err)
	}
	if !strings.Contains(status.String(), "telegram: active") {
		t.Fatalf("expected telegram active in status, got: %q", status.String())
	}
}

func TestExecute_Notify_Disable_TripsTransportMuteAndReportsIt(t *testing.T) {
	repo := testutil.TempRepo(t)
	cmdtest.StoreProjectFor(t, repo)

	var stdout bytes.Buffer
	d := Deps{Env: cmdbase.Env{
		Stdout: &stdout,
		Stderr: bytes.NewBuffer(nil),
		Getwd:  func() (string, error) { return repo, nil },
		Env:    cmdtest.EnvFor(t),
	}}

	if err := run(t, d, "notify", "--disable", "slack"); err != nil {
		t.Fatalf("execute notify --disable: %v", err)
	}
	if !strings.Contains(stdout.String(), "slack: muted") {
		t.Fatalf("expected muted report, got: %q", stdout.String())
	}

	var status bytes.Buffer
	d.Stdout = &status
	if err := run(t, d, "notify", "--status"); err != nil {
		t.Fatalf("execute notify --status: %v", err)
	}
	if !strings.Contains(status.String(), "slack: muted (manual-disable)") {
		t.Fatalf("expected slack muted (manual-disable) in status, got: %q", status.String())
	}
	if !strings.Contains(status.String(), "telegram: active") {
		t.Fatalf("expected telegram unaffected, got: %q", status.String())
	}
}

func TestExecute_Notify_EnableDisable_UnknownTransportRejected(t *testing.T) {
	d := Deps{Env: cmdbase.Env{Stdout: bytes.NewBuffer(nil), Stderr: bytes.NewBuffer(nil), Env: cmdtest.EnvFor(t)}}
	err := run(t, d, "notify", "--enable", "carrier-pigeon")
	if err == nil || !strings.Contains(err.Error(), "carrier-pigeon") {
		t.Fatalf("expected unknown-transport error, got: %v", err)
	}
}

func TestExecute_Notify_Status_ReportsTicketsWithMutes(t *testing.T) {
	dir := testutil.TempRepo(t)
	project := cmdtest.StoreProjectFor(t, dir)
	writeMutedTicket(t, project, "an-epic", "01-work.md", "01", `
  - event_type: gate-tripped
    tripped_at: 2026-08-01T00:00:00Z`)

	var stdout bytes.Buffer
	d := Deps{Env: cmdbase.Env{
		Stdout: &stdout,
		Stderr: bytes.NewBuffer(nil),
		Getwd:  func() (string, error) { return dir, nil },
		Env:    cmdtest.EnvFor(t),
	}}
	if err := run(t, d, "notify", "--status"); err != nil {
		t.Fatalf("execute notify --status: %v", err)
	}
	if !strings.Contains(stdout.String(), "an-epic/01: gate-tripped") {
		t.Fatalf("expected muted ticket line, got: %q", stdout.String())
	}
}

func TestExecute_Notify_MessageWithStatusRejected(t *testing.T) {
	t.Parallel()
	d := Deps{Env: cmdbase.Env{Stdout: bytes.NewBuffer(nil), Stderr: bytes.NewBuffer(nil)}}
	err := run(t, d, "notify", "hello", "--status")
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutual-exclusion error, got: %v", err)
	}
}

func TestExecute_Notify_MessageWithEnableRejected(t *testing.T) {
	t.Parallel()
	d := Deps{Env: cmdbase.Env{Stdout: bytes.NewBuffer(nil), Stderr: bytes.NewBuffer(nil)}}
	err := run(t, d, "notify", "hello", "--enable", "slack")
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutual-exclusion error, got: %v", err)
	}
}

func writeMutedTicket(t *testing.T, projectDir, epic, filename, id, mutesYAML string) {
	t.Helper()
	path := filepath.Join(projectDir, epic, "issues", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	cmdtest.EnsureIssueEpic(t, path)
	content := "---\nid: \"" + id + "\"\nstatus: open\ntype: implement\nmutes:" + mutesYAML + "\n---\nBody.\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestExecute_Notify_PerDestinationMutes(t *testing.T) {
	repo := testutil.TempRepo(t)
	store := cmdtest.IsolateTicketStore(t)
	cmdtest.AddProject(t, store, "plain", repo)
	own := cmdtest.AddProject(t, store, "own", repo)
	if err := os.WriteFile(filepath.Join(own, "project.json"), []byte(
		`{"name":"own","repo":"`+repo+`","notifications":{"telegram":{"bot-token":"tok","chat-id":"7"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	// A state file from before destinations existed: transport-keyed.
	statePath, err := notificationStatePath(gxconfig.DirsFrom(cmdtest.EnvFor(t)))
	if err != nil {
		t.Fatal(err)
	}
	if err := notify.UpdateNotificationState(statePath, func(s *notify.NotificationState) {
		s.Transports["slack"] = notify.TransportState{Muted: true, Reason: "manual-disable"}
	}); err != nil {
		t.Fatal(err)
	}

	runOut := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		d := Deps{Env: cmdbase.Env{Stdout: &out, Stderr: bytes.NewBuffer(nil), Getwd: func() (string, error) { return repo, nil },
			Env: cmdtest.EnvFor(t),
			LoadConfig: func() (gxconfig.Config, error) {
				cfg := gxconfig.Default()
				cfg.TicketStore.Path = store
				cfg.Notifications.Telegram = gxconfig.TelegramConfig{BotToken: "global", ChatID: "1"}
				return cfg, nil
			}}}
		if err := run(t, d, args...); err != nil {
			t.Fatalf("execute %v: %v", args, err)
		}
		return out.String()
	}

	if got := runOut("notify", "--status"); !strings.Contains(got, "slack: muted (manual-disable)") {
		t.Fatalf("old slack mute should be the global destination's, got: %q", got)
	}

	if got := runOut("notify", "--disable", "telegram", "--project", "own"); strings.Contains(got, "telegram: muted") || !strings.Contains(got, "telegram:") {
		t.Fatalf("expected only the project's destination muted, got: %q", got)
	}
	status := runOut("notify", "--status")
	if !strings.Contains(status, "telegram: active [projects: plain]") {
		t.Fatalf("global telegram should stay active with its projects, got: %q", status)
	}
	if !strings.Contains(status, "muted (manual-disable) [projects: own]") {
		t.Fatalf("project destination should be muted and list its project, got: %q", status)
	}

	if got := runOut("notify", "--enable", "telegram"); strings.Count(got, "active") != 2 {
		t.Fatalf("--enable without --project should clear every telegram destination, got: %q", got)
	}
}
