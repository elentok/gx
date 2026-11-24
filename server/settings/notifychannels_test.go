package settings

import (
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
		{"zero value, default kind", NotifyChannel{}, "", true, true},
		{"switch off", NotifyChannel{Off: true, Kinds: []string{"a"}}, "a", false, false},
		{"switch off, system kind", NotifyChannel{Off: true}, "a", true, false},
		{"kind checked", NotifyChannel{Kinds: []string{"a"}}, "a", false, true},
		{"kind unchecked", NotifyChannel{Kinds: []string{"b"}}, "a", false, false},
		{"system kind skips the list", NotifyChannel{Kinds: []string{}}, "a", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ch.Allows(tt.kind, tt.system); got != tt.want {
				t.Fatalf("Allows(%q, %v) = %v, want %v", tt.kind, tt.system, got, tt.want)
			}
		})
	}
}

func TestNotifyChannelsByID(t *testing.T) {
	n := NotifyChannels{Desktop: NotifyChannel{Off: true}, Push: NotifyChannel{Kinds: []string{"x"}}}
	if ch, ok := n.ByID(notify.ChannelDesktop); !ok || !ch.Off {
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
