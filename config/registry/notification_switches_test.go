package registry

import (
	"testing"

	"github.com/elentok/gx/notifykind"
)

// A settings editor types its controls from the registry, so every channel
// switch must be a boolean in the Notifications category.
func TestNotificationSwitches_AreBooleans(t *testing.T) {
	r := Default()
	channels := notifykind.Channels()
	if len(channels) != 5 {
		t.Fatalf("want 5 channels, got %d", len(channels))
	}
	for _, ch := range channels {
		key := Key("notification-channels", ch.ID, "enabled")
		e, ok := r.Lookup(key)
		if !ok {
			t.Errorf("%s missing from the registry", key)
			continue
		}
		if e.Type != TypeBool {
			t.Errorf("%s type = %v, want TypeBool", key, e.Type)
		}
		if e.Category != CategoryNotifications {
			t.Errorf("%s category = %v, want Notifications", key, e.Category)
		}
	}
}
