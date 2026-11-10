package prflow

// act is the per-poll dispatch: later PR tickets add their action here, each in
// its own file, and call it from this function.
func (w *Watcher) act(Target, View) {}
