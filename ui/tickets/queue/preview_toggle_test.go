package queue

import (
	"strings"
	"testing"
)

func TestPreviewToggle_ListedInHelp(t *testing.T) {
	t.Parallel()
	found := false
	for _, b := range newQueueKeysManager().Bindings() {
		if strings.Join(b.Seq, " ") == "t v" && b.Title != "" {
			found = true
		}
	}
	if !found {
		t.Error("queue: expected a titled \"t v\" binding")
	}
}
