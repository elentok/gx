package agents

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/ui/agents/terminal"
)

// ownerReport delivers a holder message on the attach and returns the model.
func holderMsg(t *testing.T, m Model, fa *fakeAttach, msg holder.AttachMessage) Model {
	t.Helper()
	fa.msgs <- msg
	m, _ = read(t, m, terminal.ReadAttach(m.attach))
	return m
}

// wideAttach is focusedAttach in a window wide enough for the whole header.
func wideAttach(t *testing.T) (Model, *fakeAttach) {
	t.Helper()
	base, fa := focusedAttach(t)
	return update1(t, base, tea.WindowSizeMsg{Width: 160, Height: 12}), fa
}

func ownerReport(t *testing.T, m Model, fa *fakeAttach, o holder.AttachOwner) Model {
	t.Helper()
	return holderMsg(t, m, fa, holder.AttachMessage{Type: holder.StreamOwner, Owner: o})
}

func TestAttach_IndicatorNamesTheOwner(t *testing.T) {
	cases := map[string]struct {
		owner      holder.AttachOwner
		want, none string
	}{
		"self":    {holder.AttachOwner{Self: true}, "input: this client", "take over"},
		"another": {holder.AttachOwner{}, "input: another client", "gx is typing"},
		"gx":      {holder.AttachOwner{GX: true}, "gx is typing", "another client"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			m, fa := wideAttach(t)
			m = ownerReport(t, m, fa, c.owner)
			view := plainView(m)
			if !strings.Contains(view, c.want) || strings.Contains(view, c.none) {
				t.Fatalf("indicator wrong, want %q without %q:\n%s", c.want, c.none, view)
			}
			if name != "self" && !strings.Contains(view, "ctrl+a t to take") {
				t.Fatalf("a non-owner shows no take-over hint:\n%s", view)
			}
		})
	}
}

func TestAttach_TakeOverKeySendsTheRequestAndFocusDoesNot(t *testing.T) {
	m, fa := focusedAttach(t)
	m = settle(m)
	if slices.Contains(fa.log, "takeover") {
		t.Fatal("focusing sent a take-over")
	}
	m = pressKeys(t, m, tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl}, letter(terminal.TakeOverKey))
	settle(m)
	if !slices.Equal(fa.log, []string{"takeover"}) || len(fa.keys) != 0 {
		t.Fatalf("log = %v, keys = %v; want one take-over and no key", fa.log, fa.keys)
	}
}

func TestAttach_BlockedKeyHintClearsOnTimeout(t *testing.T) {
	m, fa := wideAttach(t)
	m = ownerReport(t, m, fa, holder.AttachOwner{})
	m = holderMsg(t, m, fa, holder.AttachMessage{Type: holder.StreamBlocked})
	if !strings.Contains(plainView(m), "another client · blocked") {
		t.Fatalf("no blocked hint:\n%s", plainView(m))
	}
	m = update1(t, m, terminal.BlockedClearMsg{A: m.attach, Seq: 0})
	if !strings.Contains(plainView(m), "another client · blocked") {
		t.Fatal("a stale clear dropped the hint")
	}
	m = update1(t, m, terminal.BlockedClearMsg{A: m.attach, Seq: 1})
	if strings.Contains(plainView(m), "another client · blocked") {
		t.Fatalf("hint stayed after the timeout:\n%s", plainView(m))
	}
}

func TestAttach_BlockedKeyHintClearsOnTakeOver(t *testing.T) {
	m, fa := wideAttach(t)
	m = ownerReport(t, m, fa, holder.AttachOwner{})
	m = holderMsg(t, m, fa, holder.AttachMessage{Type: holder.StreamBlocked})
	m = ownerReport(t, m, fa, holder.AttachOwner{Self: true})
	view := plainView(m)
	if strings.Contains(view, "another client · blocked") || !strings.Contains(view, "this client") {
		t.Fatalf("hint stayed after take-over:\n%s", view)
	}
}
