package config

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"

	"github.com/elentok/gx/config/keyedit"
	"github.com/elentok/gx/fsutil"
	"github.com/elentok/gx/notifykind"
)

// NotificationKindsMarkerKey is the top-level key that says the one-time kind
// list migration ran, or that the file was made by a gx that needs none.
const NotificationKindsMarkerKey = "notification-kinds-migrated"

// upgradeKinds is what each channel sent before kind lists existed. Telegram
// and Slack sent everything but the (new) Long ticket.
func upgradeKinds() []struct {
	channel string
	kinds   []string
} {
	var all []string
	for _, id := range notifykind.IDs() {
		if id != notifykind.LongTicket {
			all = append(all, id)
		}
	}
	return []struct {
		channel string
		kinds   []string
	}{
		{"push", []string{notifykind.SessionAlert, notifykind.DesignQuestion, notifykind.PausedTicket}},
		{"desktop", []string{notifykind.SessionAlert, notifykind.DesignQuestion}},
		{"web-toast", []string{}},
		{"telegram", all},
		{"slack", slices.Clone(all)},
	}
}

// MigrateNotificationKinds upgrades config file bytes that have no marker:
// it writes each channel's kind list that is not set yet, then the marker.
// It returns the bytes unchanged when no migration applies (marked, not a JSON
// object) and reports changed=false then.
func MigrateNotificationKinds(data []byte) (out []byte, changed bool) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil || top == nil {
		return data, false
	}
	if _, ok := top[NotificationKindsMarkerKey]; ok {
		return data, false
	}
	var channels map[string]map[string]json.RawMessage
	if raw, ok := top["notification-channels"]; ok {
		// A block of another shape is reported by parse; leave the file alone.
		if err := json.Unmarshal(raw, &channels); err != nil {
			return data, false
		}
	}
	out = data
	for _, u := range upgradeKinds() {
		if _, set := channels[u.channel]["kinds"]; set {
			continue
		}
		var err error
		if out, err = keyedit.Set(out, []string{"notification-channels", u.channel, "kinds"}, u.kinds); err != nil {
			return data, false
		}
	}
	out, err := keyedit.Set(out, []string{NotificationKindsMarkerKey}, true)
	if err != nil {
		return data, false
	}
	return out, true
}

// ReadMigrated reads the config file at path and migrates it in place when it
// is an upgrade. A missing file is a new install: nothing is written and the
// read error is returned as is. A failed write is not fatal: the upgraded
// bytes are returned anyway, and the next load tries again.
func ReadMigrated(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out, changed := MigrateNotificationKinds(data)
	if !changed || bytes.Equal(out, data) {
		return data, nil
	}
	_ = fsutil.WriteFileAtomic(path, out) // not fatal, see above
	return out, nil
}
