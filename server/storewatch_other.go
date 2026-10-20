//go:build !(darwin && cgo)

package server

import "github.com/fsnotify/fsnotify"

// NewStoreWatch watches every directory under root with fsnotify.
func NewStoreWatch(root string) (*StoreWatch, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	WatchTree(w, root)
	go func() {
		// Drain errors so the watcher never blocks; the poll covers any loss.
		for range w.Errors {
		}
	}()
	return &StoreWatch{
		Events: w.Events,
		stop: func() {
			for _, p := range w.WatchList() {
				_ = w.Remove(p)
			}
			_ = w.Close()
		},
		rewalk: func() { WatchTree(w, root) },
	}, nil
}
