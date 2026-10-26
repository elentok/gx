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

func TestServerTicketsMergeAndStartOverReplaceLandAndReset(t *testing.T) {
	root := newRootCmd(deps{})
	for _, verb := range []string{"merge", "start-over"} {
		c, _, err := root.Find([]string{"server", "tickets", verb})
		if err != nil || c.Name() != verb {
			t.Errorf("gx server tickets %s: %v (found %v)", verb, err, c.Name())
		}
	}
	start, _, _ := root.Find([]string{"server", "tickets", "start-over"})
	if !strings.Contains(start.Short, "backup branch") {
		t.Errorf("start-over help = %q, want it to mention the backup branch", start.Short)
	}
	for _, old := range []string{"land", "reset"} {
		if c, _, _ := root.Find([]string{"server", "tickets", old}); c.Name() == old {
			t.Errorf("gx server tickets %s still exists", old)
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

func TestServerTicketsRetryHasHiddenRelaunchAlias(t *testing.T) {
	root := newRootCmd(deps{})
	for verb, hidden := range map[string]bool{"retry": false, "relaunch": true} {
		c, _, err := root.Find([]string{"server", "tickets", verb})
		if err != nil || c.Name() != verb {
			t.Errorf("gx server tickets %s: %v (found %v)", verb, err, c.Name())
			continue
		}
		if c.Hidden != hidden {
			t.Errorf("gx server tickets %s: hidden = %v, want %v", verb, c.Hidden, hidden)
		}
		if c.Flags().Lookup("stop") == nil {
			t.Errorf("gx server tickets %s has no --stop flag", verb)
		}
	}
	tickets, _, _ := root.Find([]string{"server", "tickets"})
	var help bytes.Buffer
	tickets.SetOut(&help)
	if err := tickets.Help(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(help.String(), "relaunch") {
		t.Errorf("tickets help lists relaunch:\n%s", help.String())
	}
	if !strings.Contains(help.String(), "retry") {
		t.Errorf("tickets help is missing retry:\n%s", help.String())
	}
}

func TestServerQueueDrainIsHiddenButDispatches(t *testing.T) {
	root := newRootCmd(deps{})
	c, _, err := root.Find([]string{"server", "queue", "drain"})
	if err != nil || c.Name() != "drain" {
		t.Fatalf("gx server queue drain: %v (found %v)", err, c.Name())
	}
	if !c.Hidden {
		t.Error("gx server queue drain is not hidden")
	}
	queue, _, _ := root.Find([]string{"server", "queue"})
	var help bytes.Buffer
	queue.SetOut(&help)
	if err := queue.Help(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(help.String(), "drain") {
		t.Errorf("queue help lists drain:\n%s", help.String())
	}
	if !strings.Contains(help.String(), "pause") {
		t.Errorf("queue help is missing pause:\n%s", help.String())
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
