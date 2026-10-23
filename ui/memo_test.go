package ui

import "testing"

func TestMemo_RendersOnlyWhenKeyChanges(t *testing.T) {
	var m Memo
	calls := 0
	render := func(out string) func() string {
		return func() string { calls++; return out }
	}
	type key struct{ lines []string }

	if got := m.Get(key{[]string{"a"}}, render("A")); got != "A" {
		t.Fatalf("first Get = %q, want A", got)
	}
	if got := m.Get(key{[]string{"a"}}, render("stale")); got != "A" || calls != 1 {
		t.Fatalf("same key: got %q after %d renders, want A after 1", got, calls)
	}
	if got := m.Get(key{[]string{"b"}}, render("B")); got != "B" || calls != 2 {
		t.Fatalf("new key: got %q after %d renders, want B after 2", got, calls)
	}
}

func TestMemo_NilRendersEveryTime(t *testing.T) {
	var m *Memo
	calls := 0
	for range 2 {
		m.Get("k", func() string { calls++; return "x" })
	}
	if calls != 2 {
		t.Fatalf("nil Memo rendered %d times, want 2", calls)
	}
}
