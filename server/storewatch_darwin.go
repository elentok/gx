//go:build darwin && cgo

package server

import (
	"time"

	"github.com/fsnotify/fsevents"
	"github.com/fsnotify/fsnotify"
)

// fseventsLatency coalesces kernel events before they reach us; the watch
// loop debounces on top of it.
const fseventsLatency = 20 * time.Millisecond

// NewStoreWatch watches every directory and file under root.
func NewStoreWatch(root string) (*StoreWatch, error) {
	return newStoreWatch(root, (*fsevents.EventStream).Start)
}

func newStoreWatch(root string, start func(*fsevents.EventStream) error) (*StoreWatch, error) {
	es := &fsevents.EventStream{
		Paths:   []string{root},
		Latency: fseventsLatency,
		Flags:   fsevents.FileEvents | fsevents.WatchRoot,
	}
	if err := start(es); err != nil {
		return newFSNotifyStoreWatch(root)
	}
	out := make(chan fsnotify.Event, 64)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case batch := <-es.Events:
				for _, ev := range batch {
					if inGitDir(ev.Path) {
						continue
					}
					for _, op := range fseventsOps(ev.Flags) {
						select {
						case out <- fsnotify.Event{Name: ev.Path, Op: op}:
						case <-done:
							return
						}
					}
				}
			}
		}
	}()
	return &StoreWatch{
		Events: out,
		// The stream callback sends on the unbuffered es.Events, so a batch that
		// lands after the readers left blocks its GCD thread forever; enough of
		// those starve later streams. Stop the stream while es.Events is still
		// read, then end the reader. The backstop poll in keepFresh covers any
		// event lost on darwin too.
		stop: func() {
			stopped := make(chan struct{})
			go func() {
				es.Stop()
				close(stopped)
			}()
			for {
				select {
				case <-es.Events:
				case <-stopped:
					close(done)
					return
				}
			}
		},
		rewalk: func() {},
	}, nil
}

// fseventsOps maps FSEvents flags (several can be coalesced into one event)
// to fsnotify ops. A lost-events flag maps to a Write, so the caller rescans.
func fseventsOps(f fsevents.EventFlags) []fsnotify.Op {
	var ops []fsnotify.Op
	if f&fsevents.ItemCreated != 0 {
		ops = append(ops, fsnotify.Create)
	}
	if f&fsevents.ItemRemoved != 0 {
		ops = append(ops, fsnotify.Remove)
	}
	if f&fsevents.ItemRenamed != 0 {
		ops = append(ops, fsnotify.Rename)
	}
	if len(ops) == 0 || f&fsevents.ItemModified != 0 {
		ops = append(ops, fsnotify.Write)
	}
	return ops
}
