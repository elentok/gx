package ticketwrite

import (
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/schema"
)

// RetroKeyPrefix is how RetroSubmit's dedupe key names the epic a retro covers.
const RetroKeyPrefix = "evolve:"

// RetroSubmit is the one-off a retro of epic runs as: a commitless evolve ticket
// in the epic's own project. The dedupe key refuses a second live retro of the
// same epic; the timestamped name keeps a --force re-run from hitting a taken one.
func RetroSubmit(project, epic string, now time.Time) wire.OneOffRequest {
	return wire.OneOffRequest{
		Prompt:  "Retro of epic " + project + ":" + epic,
		Project: project,
		Name:    "evolve-" + epic + "-" + now.Format("20060102-1504"),
		Type:    string(schema.TypeEvolve),
		Unique:  RetroKeyPrefix + project + "/" + epic,
		Notify:  true,
	}
}
