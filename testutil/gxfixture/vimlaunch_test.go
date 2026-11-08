package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
)

const (
	vimLaunchTicket = "demo:vim-launch-epic/01"
	// vimCheckLogFragment is the stable part of the launch-failed log line.
	vimCheckLogFragment = "vim editor mode despite the editorMode normal launch override"
)

func TestVimLaunchScenario_TheServerLogShowsTheVimCheckAndTheTicketStillFinishes(t *testing.T) {
	t.Parallel()
	r := start(t, "vim-launch")
	r.waitStatus(t, vimLaunchTicket, "done")
	var log string
	// A persistent footer fails the check on the launch and on its one relaunch.
	if !testutil.Poll(30*time.Second, 200*time.Millisecond, func() bool {
		data, err := os.ReadFile(r.LogFile)
		log = string(data)
		return err == nil && strings.Count(log, vimCheckLogFragment) >= 2
	}) {
		t.Fatalf("want at least two vim-check lines in %s, got:\n%s", r.LogFile, log)
	}
	r.stop(t)
}
