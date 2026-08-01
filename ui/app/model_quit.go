package app

// quitGuard is the duck type a page/tab can implement to block quitting gx
// (see handleBack) when it has state that shouldn't be interrupted, e.g. an
// in-flight ralph-loop.
type quitGuard interface{ CanQuit() bool }

// canQuit checks every cached tab, not just the active one, since a
// ralph-loop launched from the tickets tab keeps running in the background
// after the user switches away — it must still block quitting from any tab.
func (m Model) canQuit() bool {
	for _, page := range m.livePageByTab {
		if qg, ok := page.model.(quitGuard); ok && !qg.CanQuit() {
			return false
		}
	}
	return true
}
