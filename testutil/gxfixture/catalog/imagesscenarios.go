package catalog

import (
	"slices"

	"github.com/elentok/gx/testutil/gxfixture/spec"
)

// agentsImagesScenario is the agents scenario with saved images in the chat of
// its first Claude session and its Codex session.
func agentsImagesScenario() scenario {
	sc := spec.AgentsScenario()
	sc.Sessions = slices.Clone(sc.Sessions)
	for i := range sc.Sessions {
		if i == 0 || sc.Sessions[i].Harness == "codex" {
			sc.Sessions[i].Images = true
		}
	}
	return sc
}
