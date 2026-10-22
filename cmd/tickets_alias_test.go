package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestOldRepairVerbsAreUnknownTicketsCommands(t *testing.T) {
	for _, verb := range []string{"land", "reset", "unpark", "verify"} {
		root := newRootCmd(deps{})
		root.SetArgs([]string{"tickets", verb, "epic", "01"})
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), "unknown command") {
			t.Errorf("gx tickets %s: err = %v, want unknown command", verb, err)
		}
	}
}

func TestServerTicketsPauseAndResumeKeepHiddenAliases(t *testing.T) {
	root := newRootCmd(deps{})
	for verb, hidden := range map[string]bool{"pause": false, "resume": false, "park": true, "unpark": true} {
		c, _, err := root.Find([]string{"server", "tickets", verb})
		if err != nil || c.Name() != verb {
			t.Errorf("gx server tickets %s: %v (found %v)", verb, err, c.Name())
			continue
		}
		if c.Hidden != hidden {
			t.Errorf("gx server tickets %s: hidden = %v, want %v", verb, c.Hidden, hidden)
		}
	}
}

func TestRootHelpGroupsCommands(t *testing.T) {
	root := newRootCmd(deps{})
	var help bytes.Buffer
	root.SetOut(&help)
	if err := root.Help(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Ticket content (direct):", "Server:"} {
		if !strings.Contains(help.String(), want) {
			t.Errorf("root help missing group %q:\n%s", want, help.String())
		}
	}
}
