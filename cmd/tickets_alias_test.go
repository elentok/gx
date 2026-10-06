package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestOldRepairVerbsAreHiddenAliasesThatWarn(t *testing.T) {
	root := newRootCmd(deps{})
	tickets, _, err := root.Find([]string{"tickets"})
	if err != nil {
		t.Fatal(err)
	}
	var help bytes.Buffer
	tickets.SetOut(&help)
	if err := tickets.Help(); err != nil {
		t.Fatal(err)
	}
	for _, verb := range []string{"land", "reset", "unpark", "verify"} {
		c, _, err := root.Find([]string{"tickets", verb})
		if err != nil || c.Name() != verb {
			t.Fatalf("tickets %s not found: %v", verb, err)
		}
		if !c.Hidden {
			t.Errorf("tickets %s should be hidden", verb)
		}
		if strings.Contains(help.String(), "  "+verb+" ") {
			t.Errorf("tickets help lists %s:\n%s", verb, help.String())
		}
	}
}

func TestDeprecatedRepairAliasWarnsThenRuns(t *testing.T) {
	ran := false
	c := deprecatedRepairAlias(&cobra.Command{Use: "land", RunE: func(*cobra.Command, []string) error { ran = true; return nil }})
	var stderr bytes.Buffer
	c.SetErr(&stderr)
	if err := c.RunE(c, nil); err != nil || !ran {
		t.Fatalf("ran=%v err=%v", ran, err)
	}
	if !strings.Contains(stderr.String(), "gx server tickets land") {
		t.Errorf("warning = %q", stderr.String())
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
