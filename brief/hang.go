package brief

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// hangWatcher reads the `go test -json` stream as it is written and reports the
// first test that has been running longer than limit. It tracks each test's own
// start, as parallel tests overlap. A paused parallel test waits for others, so
// its clock restarts at "cont".
type hangWatcher struct {
	limit time.Duration

	mu      sync.Mutex
	partial []byte
	started map[hangKey]time.Time
	hung    *hangKey
	stop    chan struct{}
}

type hangKey struct{ pkg, test string }

func newHangWatcher(limit time.Duration) *hangWatcher {
	return &hangWatcher{limit: limit, started: map[hangKey]time.Time{}, stop: make(chan struct{})}
}

func (h *hangWatcher) Write(p []byte) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.partial = append(h.partial, p...)
	for {
		i := bytes.IndexByte(h.partial, '\n')
		if i < 0 {
			return len(p), nil
		}
		h.event(h.partial[:i])
		h.partial = h.partial[i+1:]
	}
}

func (h *hangWatcher) event(line []byte) {
	var ev goTestEvent
	if len(line) == 0 || line[0] != '{' || json.Unmarshal(line, &ev) != nil || ev.Test == "" {
		return
	}
	k := hangKey{ev.Package, ev.Test}
	switch ev.Action {
	case "run", "cont":
		h.started[k] = time.Now()
	case "pause", "pass", "fail", "skip":
		delete(h.started, k)
	}
}

// watch closes the stop channel when a test passes the limit, until done closes.
func (h *hangWatcher) watch(done <-chan struct{}) {
	tick := time.NewTicker(max(h.limit/10, 20*time.Millisecond))
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-tick.C:
			if h.check(time.Now()) {
				return
			}
		}
	}
}

func (h *hangWatcher) check(now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	var oldest *hangKey
	var at time.Time
	for k, t := range h.started {
		if now.Sub(t) > h.limit && (oldest == nil || t.Before(at)) {
			k := k
			oldest, at = &k, t
		}
	}
	if oldest == nil {
		return false
	}
	h.hung = oldest
	close(h.stop)
	return true
}

// killedLines are the events appended to a killed run's output, so that Parse
// names the hung test like any failed one. The interrupted test never reports.
func (h *hangWatcher) killedLines() []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hung == nil {
		return nil
	}
	k := *h.hung
	msg := fmt.Sprintf("HUNG: %s ran longer than %s and was killed\n", k.test, h.limit)
	var b bytes.Buffer
	for _, ev := range []goTestEvent{{Action: "output", Output: msg}, {Action: "fail"}} {
		ev.Package, ev.Test = k.pkg, k.test
		line, _ := json.Marshal(ev)
		b.Write(append(line, '\n'))
	}
	return b.Bytes()
}
