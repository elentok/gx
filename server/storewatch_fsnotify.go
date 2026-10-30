package server

import "github.com/fsnotify/fsnotify"

// newFSNotifyStoreWatch is also the fallback when FSEvents cannot start.
func newFSNotifyStoreWatch(root string) (*StoreWatch, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	WatchTree(w, root)
	done := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		// Flush errors so the watcher never blocks; the poll covers any loss.
		for {
			select {
			case <-done:
				return
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	return &StoreWatch{
		Events: w.Events,
		stop: func() {
			for _, p := range w.WatchList() {
				_ = w.Remove(p)
			}
			_ = w.Close()
			close(done)
			<-exited
		},
		rewalk: func() { WatchTree(w, root) },
	}, nil
}
