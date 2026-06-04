package reloadgate

import "github.com/elentok/gx/ui/nav"

// ReloadGate tracks a global repo epoch and per-tab load stamps so the app
// shell can decide whether a cached tab needs an auto-reload on activation.
//
// Zero-value behavior: a fresh gate with epoch 0 reports no tab as stale —
// only after the first Mutated() call do un-stamped tabs become stale.
type ReloadGate struct {
	epoch       uint64
	loadedByTab map[nav.TabID]uint64
}

func New() *ReloadGate {
	return &ReloadGate{
		loadedByTab: make(map[nav.TabID]uint64),
	}
}

// Mutated bumps the global epoch. Call once per completed mutating git op.
func (g *ReloadGate) Mutated() {
	g.epoch++
}

// MarkLoaded stamps tab as fresh at the current epoch.
func (g *ReloadGate) MarkLoaded(tab nav.TabID) {
	g.loadedByTab[tab] = g.epoch
}

// ShouldAutoReload reports true when the tab's last-loaded epoch is behind the
// current epoch, meaning a mutation occurred since it was last loaded.
func (g *ReloadGate) ShouldAutoReload(tab nav.TabID) bool {
	return g.loadedByTab[tab] < g.epoch
}
