package settings

import (
	"slices"
	"testing"

	"github.com/elentok/gx/ralphloop/notify"
)

func TestNotifyChannelAllows(t *testing.T) {
	tests := []struct {
		name   string
		ch     NotifyChannel
		kind   string
		system bool
		want   bool
	}{
		{"zero value is off", NotifyChannel{}, "a", true, false},
		{"switch off", NotifyChannel{Kinds: []string{"a"}}, "a", false, false},
		{"switch off, system kind", NotifyChannel{}, "a", true, false},
		{"kind checked", NotifyChannel{Enabled: true, Kinds: []string{"a"}}, "a", false, true},
		{"kind unchecked", NotifyChannel{Enabled: true, Kinds: []string{"b"}}, "a", false, false},
		{"nil list checks no kind", NotifyChannel{Enabled: true}, "a", false, false},
		{"system kind skips the list", NotifyChannel{Enabled: true, Kinds: []string{}}, "a", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ch.Allows(tt.kind, tt.system); got != tt.want {
				t.Fatalf("Allows(%q, %v) = %v, want %v", tt.kind, tt.system, got, tt.want)
			}
		})
	}
}

func TestNotifyChannelKindIDsReturnsKindsAsStored(t *testing.T) {
	if got := (NotifyChannel{}).KindIDs(); got != nil {
		t.Errorf("nil Kinds = %v, want nil", got)
	}
	if got := (NotifyChannel{Kinds: []string{}}).KindIDs(); got == nil || len(got) != 0 {
		t.Errorf("empty Kinds = %#v, want an empty non-nil list", got)
	}
	if got := (NotifyChannel{Kinds: []string{"a", "b"}}).KindIDs(); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("Kinds = %v, want [a b]", got)
	}
}

func TestNotifyChannelsByID(t *testing.T) {
	n := NotifyChannels{Desktop: NotifyChannel{}, Push: NotifyChannel{Enabled: true, Kinds: []string{"x"}}}
	if ch, ok := n.ByID(notify.ChannelDesktop); !ok || ch.Enabled {
		t.Fatalf("desktop = %+v, %v", ch, ok)
	}
	if ch, ok := n.ByID(notify.ChannelPush); !ok || len(ch.Kinds) != 1 {
		t.Fatalf("push = %+v, %v", ch, ok)
	}
	for _, id := range []string{notify.ChannelWebToast, notify.ChannelTelegram, notify.ChannelSlack} {
		if _, ok := n.ByID(id); !ok {
			t.Fatalf("%s not found", id)
		}
	}
	if _, ok := n.ByID("nope"); ok {
		t.Fatal("unknown id found")
	}
}
