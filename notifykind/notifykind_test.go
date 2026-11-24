package notifykind

import (
	"slices"
	"strings"
	"testing"
)

func TestCatalog_SixteenUniqueLowercaseIDs(t *testing.T) {
	all := All()
	if len(all) != 16 {
		t.Fatalf("catalog has %d rows, want 16", len(all))
	}
	seen := map[string]bool{}
	for _, k := range all {
		if k.ID != strings.ToLower(k.ID) || strings.ContainsAny(k.ID, " _") {
			t.Errorf("id %q is not lower-case kebab", k.ID)
		}
		if seen[k.ID] {
			t.Errorf("duplicate id %q", k.ID)
		}
		seen[k.ID] = true
		if k.Name == "" || k.Hint == "" {
			t.Errorf("%s has an empty name or hint", k.ID)
		}
	}
}

func TestCatalog_OnlyLongTicketIsUncheckedByDefault(t *testing.T) {
	var unchecked []string
	for _, k := range All() {
		if !k.Default {
			unchecked = append(unchecked, k.ID)
		}
	}
	if !slices.Equal(unchecked, []string{LongTicket}) {
		t.Errorf("unchecked by default = %v, want [long-ticket]", unchecked)
	}
	if got := len(DefaultIDs()); got != 15 {
		t.Errorf("DefaultIDs has %d ids, want 15", got)
	}
}

func TestValid(t *testing.T) {
	if !Valid(SessionAlert) || Valid("nope") {
		t.Error("Valid does not tell catalog ids from others")
	}
}
