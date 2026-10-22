package schema

import (
	"strings"
	"testing"
)

func TestMarshalTicket_WritesOnlyTheNewPauseForm(t *testing.T) {
	for _, status := range []Status{StatusLegacyAnswer, StatusLegacyRepair} {
		out, err := MarshalTicket(Ticket{ID: "01", Status: status, Type: TypeImplement, PauseKind: "land-conflict"}, "")
		if err != nil {
			t.Fatal(err)
		}
		got := string(out)
		if !strings.Contains(got, "status: paused") || !strings.Contains(got, "pause_reason:") {
			t.Errorf("%s: want paused with a reason, got:\n%s", status, got)
		}
		for _, banned := range []string{"park_kind", "pause_reason_text", "needs-answer", "needs-repair"} {
			if strings.Contains(got, banned) {
				t.Errorf("%s: wrote %q:\n%s", status, banned, got)
			}
		}
	}
}
