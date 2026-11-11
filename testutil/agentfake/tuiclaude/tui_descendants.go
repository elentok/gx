package tuiclaude

import . "github.com/elentok/gx/testutil/agentfake"

func (t *tui) spawnDescendants(count, depth int) error {
	return SpawnDescendantsInto(&t.descendants, count, depth)
}

func (t *tui) stopDescendants() { StopDescendantsIn(&t.descendants) }
