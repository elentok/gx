package ralphloop

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"

	"github.com/elentok/gx/config"
)

// maxTripsPerTransport bounds NotificationState's Trips history so the state
// file can't grow unbounded across a long-lived install.
const maxTripsPerTransport = 20

// lockTimeout is how long UpdateNotificationState waits for the on-disk lock
// before giving up and failing open (see ErrNotificationStateLocked).
const lockTimeout = 1 * time.Second

// ErrNotificationStateLocked is UpdateNotificationState's fail-open signal:
// another process held the lock past lockTimeout. Ticket 02 only owns
// exposing this cleanly — deciding what "proceed without the write" means
// for a notification send belongs to whatever calls this module later.
var ErrNotificationStateLocked = errors.New("notification state: lock timed out, failing open")

// NotificationEvent is one entry in a transport's trailing event series
// (pre-batch): every time the throttle gate is asked about an
// (eventType, source) pair, regardless of whether a send ever results.
type NotificationEvent struct {
	EventType string    `json:"event_type"`
	Source    string    `json:"source"`
	Time      time.Time `json:"time"`
}

// NotificationSend is one entry in a transport's trailing send series
// (post-batch): one entry per actual wire send, batched or immediate.
type NotificationSend struct {
	Time time.Time `json:"time"`
}

// SourceCount is one source's share of a trip's attributed window, used by
// TransportTrip.Sources.
type SourceCount struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}

// TransportTrip is one historical mute trip, keeping the full per-source
// breakdown of the window that caused it (every sender active in the
// window, muted or not) regardless of which sources individually tripped.
type TransportTrip struct {
	TrippedAt time.Time     `json:"tripped_at"`
	Reason    string        `json:"reason"`
	Sources   []SourceCount `json:"sources"`
}

// TransportState is one transport's (Telegram/Slack) durable throttle state.
type TransportState struct {
	Muted     bool      `json:"muted"`
	TrippedAt time.Time `json:"tripped_at,omitzero"`
	// Reason is "auto-trip" or "manual-disable".
	Reason string              `json:"reason,omitempty"`
	Events []NotificationEvent `json:"events"`
	Sends  []NotificationSend  `json:"sends"`
	Trips  []TransportTrip     `json:"trips"`
}

// NotificationState is the on-disk shape of notifications-state.json, keyed
// by transport name ("telegram", "slack") — the same strings SendMessage
// reports as sent-to.
type NotificationState struct {
	Transports map[string]TransportState `json:"transports"`
}

func emptyNotificationState() NotificationState {
	return NotificationState{Transports: map[string]TransportState{}}
}

// notificationStateFilePath returns notifications-state.json's path,
// mirroring queue-state.json's ~/.config/gx/ layout (queueStateFilePath)
// rather than introducing a new state-directory convention.
func notificationStateFilePath() (string, error) {
	base, err := config.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "gx", "notifications-state.json"), nil
}

// LoadNotificationState reads notifications-state.json, returning an empty
// state (Telegram/Slack both unmuted, empty series/history) if the file
// doesn't exist yet or fails to parse.
func LoadNotificationState() (NotificationState, error) {
	path, err := notificationStateFilePath()
	if err != nil {
		return emptyNotificationState(), err
	}
	return loadNotificationStateAt(path)
}

func loadNotificationStateAt(path string) (NotificationState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyNotificationState(), nil
		}
		return emptyNotificationState(), err
	}
	var state NotificationState
	if err := json.Unmarshal(data, &state); err != nil {
		return emptyNotificationState(), err
	}
	if state.Transports == nil {
		state.Transports = map[string]TransportState{}
	}
	return state, nil
}

// UpdateNotificationState locks notifications-state.json, loads the current
// state, applies mutate, trims every transport's Trips to the most recent
// maxTripsPerTransport entries, and writes the result back atomically.
//
// If the lock isn't acquired within lockTimeout, it returns
// ErrNotificationStateLocked without calling mutate or writing anything —
// the fail-open signal callers check for instead of blocking indefinitely.
func UpdateNotificationState(mutate func(*NotificationState)) error {
	path, err := notificationStateFilePath()
	if err != nil {
		return err
	}
	return updateNotificationStateAt(path, mutate)
}

func updateNotificationStateAt(path string, mutate func(*NotificationState)) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	lock, acquired, err := acquireNotificationStateLock(path)
	if err != nil {
		return err
	}
	if !acquired {
		return ErrNotificationStateLocked
	}
	defer lock.Close()

	state, err := loadNotificationStateAt(path)
	if err != nil {
		return err
	}
	mutate(&state)
	capTrips(&state)

	return writeNotificationStateAtomically(path, state)
}

func capTrips(state *NotificationState) {
	for transport, ts := range state.Transports {
		if len(ts.Trips) > maxTripsPerTransport {
			ts.Trips = ts.Trips[len(ts.Trips)-maxTripsPerTransport:]
			state.Transports[transport] = ts
		}
	}
}

// acquireNotificationStateLock takes an exclusive flock on path+".lock",
// polling until lockTimeout. A false, nil return means the timeout elapsed
// without acquiring the lock (fail open); a non-nil error means something
// other than contention went wrong (e.g. the lock file couldn't be opened).
func acquireNotificationStateLock(path string) (*os.File, bool, error) {
	lockFile, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, false, err
	}

	deadline := time.Now().Add(lockTimeout)
	for {
		err := unix.Flock(int(lockFile.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return lockFile, true, nil
		}
		if err != unix.EWOULDBLOCK {
			lockFile.Close()
			return nil, false, err
		}
		if time.Now().After(deadline) {
			lockFile.Close()
			return nil, false, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// writeNotificationStateAtomically replaces path's content via a
// same-directory temp file plus rename, matching tickets/schema/write.go's
// writeFileAtomic and queue_state.go's writeQueueStateAtomically so a
// concurrent reader never observes a torn/truncated write.
func writeNotificationStateAtomically(path string, state NotificationState) error {
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0644); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}
