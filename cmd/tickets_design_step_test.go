package cmd

import (
	"os"
	"strings"
	"testing"
)

func TestTicketsSet_DesignStep(t *testing.T) {
	setMap := func(t *testing.T) (deps, string) {
		d, ticketMD := setupEpicSetProject(t)
		if err := execute([]string{"tickets", "set", "mine:e", "--kind=map"}, d); err != nil {
			t.Fatal(err)
		}
		return d, ticketMD
	}

	t.Run("stores a valid step", func(t *testing.T) {
		d, ticketMD := setMap(t)
		if err := execute([]string{"tickets", "set", "mine:e", "--design-step=review"}, d); err != nil {
			t.Fatalf("set: %v", err)
		}
		if raw, _ := os.ReadFile(ticketMD); !strings.Contains(string(raw), "design_step: review") {
			t.Errorf("design_step not stored:\n%s", raw)
		}
	})

	t.Run("pings the server with the epic address", func(t *testing.T) {
		d, _ := setMap(t)
		cl, pings := fakeMoveServer(t, nil, nil)
		d.serverClient = cl
		if err := execute([]string{"tickets", "set", "mine:e", "--design-step=review"}, d); err != nil {
			t.Fatalf("set: %v", err)
		}
		if len(*pings) != 1 || (*pings)[0] != "mine:e" {
			t.Errorf("pings = %v, want [mine:e]", *pings)
		}
	})

	t.Run("refuses an unknown step and writes nothing", func(t *testing.T) {
		d, ticketMD := setMap(t)
		before, _ := os.ReadFile(ticketMD)
		if err := execute([]string{"tickets", "set", "mine:e", "--design-step=nope"}, d); err == nil {
			t.Fatal("want an error")
		}
		if after, _ := os.ReadFile(ticketMD); string(after) != string(before) {
			t.Errorf("ticket.md changed:\n%s", after)
		}
	})

	t.Run("backward move needs the flag", func(t *testing.T) {
		d, ticketMD := setMap(t)
		if err := execute([]string{"tickets", "set", "mine:e", "--design-step=review"}, d); err != nil {
			t.Fatal(err)
		}
		err := execute([]string{"tickets", "set", "mine:e", "--design-step=resolve-tickets"}, d)
		if err == nil || !strings.Contains(err.Error(), "--allow-backward") {
			t.Fatalf("err = %v, want a refusal naming --allow-backward", err)
		}
		if raw, _ := os.ReadFile(ticketMD); !strings.Contains(string(raw), "design_step: review") {
			t.Errorf("refused move changed the step:\n%s", raw)
		}
		if err := execute([]string{"tickets", "set", "mine:e", "--design-step=resolve-tickets", "--allow-backward"}, d); err != nil {
			t.Fatalf("with flag: %v", err)
		}
		if raw, _ := os.ReadFile(ticketMD); !strings.Contains(string(raw), "design_step: resolve-tickets") {
			t.Errorf("backward move not stored:\n%s", raw)
		}
	})

	t.Run("refused on a loop epic", func(t *testing.T) {
		d, ticketMD := setupEpicSetProject(t)
		if err := execute([]string{"tickets", "set", "mine:e", "--design-step=spec"}, d); err == nil {
			t.Fatal("want an error")
		}
		if raw, _ := os.ReadFile(ticketMD); string(raw) != epicTicketMD {
			t.Errorf("ticket.md changed:\n%s", raw)
		}
	})
}
