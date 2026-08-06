// Package notifylog keeps an in-memory record of every shell notification
// event, independent of ui/notify's own display lifecycle (which expires
// and caps entries for the on-screen stack).
package notifylog

import (
	"time"

	"github.com/elentok/gx/ui/notify"
)

// Capacity is the maximum number of entries retained. Appending past it
// evicts the oldest entry first (ring-buffer semantics).
const Capacity = 2000

// Entry is a single row in the notification log.
type Entry struct {
	Time    time.Time
	Kind    notify.NotifyKind
	Message string
	Closed  bool // true for a KindProgress notification's closing row
}

// Log is an in-memory, capacity-bounded record of notification events.
// The zero value is not usable; construct with New.
type Log struct {
	entries  []Entry
	progress map[string]string // id -> message, for the eventual closing row
}

func New() *Log {
	return &Log{progress: make(map[string]string)}
}

// Append records a notification event. A KindProgress event records its
// started row here; call Close with the same ID to record its closed row.
// Every other kind records a single row.
func (l *Log) Append(msg notify.NotifyMsg) {
	l.add(Entry{Time: time.Now(), Kind: msg.Kind, Message: msg.Message})
	if msg.Kind == notify.KindProgress {
		l.progress[msg.ID] = msg.Message
	}
}

// Close records the closed row for a previously-appended KindProgress
// notification. IDs with no matching started row are ignored.
func (l *Log) Close(id string) {
	message, ok := l.progress[id]
	if !ok {
		return
	}
	delete(l.progress, id)
	l.add(Entry{Time: time.Now(), Kind: notify.KindProgress, Message: message, Closed: true})
}

func (l *Log) add(e Entry) {
	l.entries = append(l.entries, e)
	if len(l.entries) > Capacity {
		l.entries = l.entries[len(l.entries)-Capacity:]
	}
}

// Entries returns the current log rows, oldest first.
func (l *Log) Entries() []Entry {
	return l.entries
}
