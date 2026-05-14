package status

import "github.com/elentok/gx/ui/keys"

func (m *Model) KeyManager() *keys.Manager {
	return &m.keys
}
