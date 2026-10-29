package servertest

import (
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

const (
	// plentyFree is what a path with no entry reports: far above any threshold.
	plentyFree = 1 << 40
	// defaultTotal is the size a disk reports unless SetTotal says otherwise.
	defaultTotal = 2 * plentyFree
)

// FakeDisks is a free-space source a test steers per path between readings.
// A path with no entry reports one shared disk with plenty of space.
type FakeDisks struct {
	mu     sync.Mutex
	byPath map[string]server.DiskReading
	reads  int
}

// Put sets what path reports: its disk id, display name and free bytes.
func (f *FakeDisks) Put(path, id, name string, free uint64) {
	f.PutSized(path, id, name, free, max(defaultTotal, free))
}

// PutSized is Put with the disk's total bytes, set in one step so no reading
// sees the new free bytes against the old total.
func (f *FakeDisks) PutSized(path, id, name string, free, total uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.byPath == nil {
		f.byPath = map[string]server.DiskReading{}
	}
	f.byPath[path] = server.DiskReading{ID: id, Name: name, Free: free, Total: total}
}

// SetTotal changes path's total bytes and keeps the rest of its reading.
func (f *FakeDisks) SetTotal(path string, total uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.read(path)
	r.Total = total
	if f.byPath == nil {
		f.byPath = map[string]server.DiskReading{}
	}
	f.byPath[path] = r
}

// SetFree changes path's free bytes and keeps its disk id and name.
func (f *FakeDisks) SetFree(path string, free uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.read(path)
	r.Free = free
	if f.byPath == nil {
		f.byPath = map[string]server.DiskReading{}
	}
	f.byPath[path] = r
}

// Read is the server.Config FreeSpace source.
func (f *FakeDisks) Read(path string) (server.DiskReading, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	return f.read(path), nil
}

// Reads is how many times the server has read a path.
func (f *FakeDisks) Reads() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads
}

// WaitReads waits until the server has read n more times than it had when the
// call began. A reading taken after a change is the event an unwanted effect
// of that change would follow, so a test checks for its absence once this returns.
func (f *FakeDisks) WaitReads(t *testing.T, n int) {
	t.Helper()
	want := f.Reads() + n
	WaitFor(t, 5*time.Second, "the watcher to read again", func() bool { return f.Reads() >= want })
}

func (f *FakeDisks) read(path string) server.DiskReading {
	if r, ok := f.byPath[path]; ok {
		return r
	}
	return server.DiskReading{ID: "fake", Name: "/fake", Free: plentyFree, Total: defaultTotal}
}
