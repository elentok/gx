//go:build darwin && cgo

package index

import (
	"errors"
	"testing"
	"time"

	"github.com/fsnotify/fsevents"
	"github.com/fsnotify/fsnotify"
)

func TestStoreWatchFakeStreamReportsTicketsAndDropsGit(t *testing.T) {
	t.Parallel()
	root := "/store"
	stream := make(chan []fsevents.Event)
	w, err := newStoreWatch(root, func(es *fsevents.EventStream) error {
		es.Events = stream
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	stream <- []fsevents.Event{
		{Path: root + "/.git/objects/x", Flags: fsevents.ItemCreated},
		{Path: root + "/epic/01-a.md", Flags: fsevents.ItemCreated},
	}
	// The timeout only bounds a failure; a passing run never waits on it.
	select {
	case ev := <-w.Events:
		if ev.Name != root+"/epic/01-a.md" || ev.Op != fsnotify.Create {
			t.Fatalf("first event = %v, want Create of the ticket file (.git dropped)", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fake stream event was not forwarded")
	}
}

func TestStoreWatchFallsBackWhenFSEventsCannotStart(t *testing.T) {
	t.Parallel()
	called := false
	testStoreWatchSeesNewFilesAndSkipsGit(t, func(root string) (*StoreWatch, error) {
		return newStoreWatch(root, func(es *fsevents.EventStream) error {
			called = true
			if len(es.Paths) != 1 || es.Paths[0] != root {
				t.Fatalf("FSEvents paths = %v, want [%s]", es.Paths, root)
			}
			return errors.New("failed to start eventstream")
		})
	})
	if !called {
		t.Fatal("FSEvents start was not attempted")
	}
}
