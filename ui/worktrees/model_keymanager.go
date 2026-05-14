package worktrees

import keysmgr "github.com/elentok/gx/ui/keys"

func (m *Model) KeyManager() *keysmgr.Manager {
	return &m.keyManager
}
