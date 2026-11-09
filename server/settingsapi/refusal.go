package settingsapi

import (
	"strings"

	"github.com/elentok/gx/config/registry"
)

// Refusal is a change the writer will not save. Reason is the text shown after
// "Not saved: ".
type Refusal struct {
	Keys   []registry.KeyPath
	Reason string
}

func (r *Refusal) Error() string {
	names := make([]string, len(r.Keys))
	for i, k := range r.Keys {
		names[i] = k.String()
	}
	return strings.Join(names, ", ") + ": " + r.Reason
}

func refuse(reason string, keys ...registry.KeyPath) *Refusal {
	return &Refusal{Keys: keys, Reason: reason}
}
