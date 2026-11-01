package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func calibrationStore(t *testing.T, files map[string]string) string {
	t.Helper()
	store := isolateTicketStore(t)
	epic := filepath.Join(store, "proj", "epic")
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(store, "proj", "project.json"), "{}")
	write(filepath.Join(epic, "ticket.md"), "---\nstatus: open\n---\n\n# epic\n")
	for name, fm := range files {
		write(filepath.Join(epic, "issues", name), "---\n"+fm+"---\nBody.\n")
	}
	return t.TempDir()
}

func runCalibration(t *testing.T, files map[string]string, asJSON bool) string {
	t.Helper()
	cwd := calibrationStore(t, files)
	args := []string{"tickets", "calibration", "proj:epic"}
	if asJSON {
		args = append(args, "--json")
	}
	out, err := runIn(t, cwd, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

const calStamped = "status: done\ntype: implement\nexpected_context_window: 20000\nprefix_tokens: 40000\n"

func TestCalibration_MedianCountAndRows(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		"01-a.md": "id: \"01\"\n" + calStamped + "actual_context_window: 50000\n",  // work 10K, 0.50
		"02-b.md": "id: \"02\"\n" + calStamped + "actual_context_window: 60000\n",  // work 20K, 1.00
		"03-c.md": "id: \"03\"\n" + calStamped + "actual_context_window: 100000\n", // work 60K, 3.00
		// skipped: commitless, stub window, zero work, not done
		"04-d.md": "id: \"04\"\n" + calStamped + "actual_context_window: 90000\ncommitless: true\n",
		"05-e.md": "id: \"05\"\nstatus: done\ntype: implement\nprefix_tokens: 40000\nactual_context_window: 90000\n",
		"06-f.md": "id: \"06\"\n" + calStamped + "actual_context_window: 30000\n",
		"07-g.md": "id: \"07\"\n" + strings.Replace(calStamped, "done", "open", 1) + "actual_context_window: 90000\n",
	}
	text := runCalibration(t, files, false)
	for _, want := range []string{"1.00", "finished tickets: 3", "01 expected=20000 work=10000", "03 expected=20000 work=60000"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	for _, bad := range []string{"approximate", "04 ", "05 ", "06 ", "07 "} {
		if strings.Contains(text, bad) {
			t.Errorf("unexpected %q in:\n%s", bad, text)
		}
	}

	var c Calibration
	if err := json.Unmarshal([]byte(runCalibration(t, files, true)), &c); err != nil {
		t.Fatal(err)
	}
	if c.Count != 3 || c.Ratio != 1 || len(c.Tickets) != 3 || c.Approximate {
		t.Errorf("json = %+v", c)
	}
}

func TestCalibration_NoFinishedTickets(t *testing.T) {
	t.Parallel()
	text := runCalibration(t, map[string]string{"01-a.md": "id: \"01\"\nstatus: open\ntype: implement\n"}, false)
	if strings.TrimSpace(text) != "no finished tickets, use 0.83" {
		t.Errorf("got %q", text)
	}
}

func TestCalibration_ApproximateWithFallbackPrefix(t *testing.T) {
	t.Parallel()
	files := map[string]string{"01-a.md": "id: \"01\"\nstatus: done\ntype: implement\nexpected_context_window: 20000\nactual_context_window: 98000\n"}
	if text := runCalibration(t, files, false); !strings.Contains(text, "approximate") {
		t.Errorf("want approximate in:\n%s", text)
	}
	var c Calibration
	if err := json.Unmarshal([]byte(runCalibration(t, files, true)), &c); err != nil {
		t.Fatal(err)
	}
	if !c.Approximate {
		t.Errorf("json = %+v", c)
	}
}
