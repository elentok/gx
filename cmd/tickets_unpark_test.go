package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
)

const parkedTicket = "---\nid: \"01\"\nstatus: needs-answer\ntype: implement\n---\n# A\n\n## Needs Answer\n\nneeds a human\n"

// uniqueEpicPath names the project dir after the test: the land lock is keyed
// by project and epic name, and every t.TempDir() parent is called "001".
func uniqueEpicPath(t *testing.T) string {
	t.Helper()
	project := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	return filepath.Join(t.TempDir(), project, "widget-epic")
}

func unparkFixture(t *testing.T, content string) (epicPath, ticketPath string) {
	t.Helper()
	epicPath = uniqueEpicPath(t)
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatal(err)
	}
	ticketPath = filepath.Join(issuesDir, "01-a.md")
	if err := os.WriteFile(ticketPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return epicPath, ticketPath
}

func TestRunTicketsUnpark_SameFileAsMenuAction(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	epicPath, ticketPath := unparkFixture(t, parkedTicket)
	_, menuPath := unparkFixture(t, parkedTicket)
	if err := ralphloop.UnparkTicket(menuPath, now); err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile(menuPath)

	var stdout, stderr bytes.Buffer
	if err := runTicketsUnpark(epicPath, "01", false, now, &stdout, &stderr); err != nil {
		t.Fatalf("err = %v", err)
	}
	got, _ := os.ReadFile(ticketPath)
	if string(got) != string(want) {
		t.Errorf("file =\n%s\nwant\n%s", got, want)
	}
}

func TestRunTicketsUnpark_JSONSuccess(t *testing.T) {
	t.Parallel()
	epicPath, _ := unparkFixture(t, parkedTicket)
	var stdout, stderr bytes.Buffer
	if err := runTicketsUnpark(epicPath, "01", true, time.Now(), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var res map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("stdout %q: %v", stdout.String(), err)
	}
	if res["ticket"] != "01" || res["status"] != "open" {
		t.Errorf("result = %v", res)
	}
}

func TestRunTicketsUnpark_RefusesNotParked(t *testing.T) {
	t.Parallel()
	epicPath, ticketPath := unparkFixture(t, "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n")
	before, _ := os.ReadFile(ticketPath)

	var stdout, stderr bytes.Buffer
	err := runTicketsUnpark(epicPath, "01", true, time.Now(), &stdout, &stderr)
	assertExit1(t, err)
	var env RefusalEnvelope
	if jerr := json.Unmarshal(stdout.Bytes(), &env); jerr != nil {
		t.Fatal(jerr)
	}
	if !env.Refused || env.Reason != ReasonNotParked {
		t.Errorf("envelope = %+v", env)
	}
	after, _ := os.ReadFile(ticketPath)
	if string(after) != string(before) {
		t.Error("refused unpark modified the file")
	}
}

func TestRunTicketsUnpark_UnknownTicket(t *testing.T) {
	t.Parallel()
	epicPath, _ := unparkFixture(t, parkedTicket)
	var stdout, stderr bytes.Buffer
	assertExit1(t, runTicketsUnpark(epicPath, "09", false, time.Now(), &stdout, &stderr))
	if stderr.Len() == 0 {
		t.Error("want message on stderr")
	}
}
