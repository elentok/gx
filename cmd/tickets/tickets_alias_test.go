package tickets

import (
	"strings"
	"testing"
)

func TestOldRepairVerbsAreUnknownTicketsCommands(t *testing.T) {
	for _, verb := range []string{"land", "reset", "unpark", "verify"} {
		err := execute(t, []string{"tickets", verb, "epic", "01"}, testDeps(t, nil, nil, nil))
		if err == nil || !strings.Contains(err.Error(), "unknown command") {
			t.Errorf("gx tickets %s: err = %v, want unknown command", verb, err)
		}
	}
}
