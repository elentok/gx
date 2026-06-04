package reloadgate_test

import (
	"testing"

	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/reloadgate"
)

func TestFreshGate_NoReload(t *testing.T) {
	g := reloadgate.New()
	for _, tab := range []nav.TabID{nav.TabLog, nav.TabStatus, nav.TabStash, nav.TabWorktrees} {
		if g.ShouldAutoReload(tab) {
			t.Errorf("fresh gate: ShouldAutoReload(%q) = true, want false", tab)
		}
	}
}

func TestMutated_UnstampedTabBecomesStale(t *testing.T) {
	g := reloadgate.New()
	g.Mutated()
	for _, tab := range []nav.TabID{nav.TabLog, nav.TabStatus} {
		if !g.ShouldAutoReload(tab) {
			t.Errorf("after Mutated: ShouldAutoReload(%q) = false, want true", tab)
		}
	}
}

func TestMarkLoaded_ClearsStalenessForThatTabOnly(t *testing.T) {
	g := reloadgate.New()
	g.Mutated()
	g.MarkLoaded(nav.TabLog)

	if g.ShouldAutoReload(nav.TabLog) {
		t.Error("after MarkLoaded(TabLog): ShouldAutoReload(TabLog) = true, want false")
	}
	if !g.ShouldAutoReload(nav.TabStatus) {
		t.Error("after MarkLoaded(TabLog): ShouldAutoReload(TabStatus) = false, want true")
	}
}

func TestActivePageFresh_OtherTabsStale(t *testing.T) {
	// Simulates: active tab (status) mutates repo; stamp it fresh; others stay stale.
	g := reloadgate.New()
	g.Mutated()
	g.MarkLoaded(nav.TabStatus) // active tab self-reload trust invariant

	if g.ShouldAutoReload(nav.TabStatus) {
		t.Error("active tab should be fresh after MarkLoaded")
	}
	for _, tab := range []nav.TabID{nav.TabLog, nav.TabStash, nav.TabWorktrees} {
		if !g.ShouldAutoReload(tab) {
			t.Errorf("inactive tab %q should be stale after mutation", tab)
		}
	}
}

func TestMarkLoaded_MultipleMutations(t *testing.T) {
	// MarkLoaded after first mutation; second mutation makes it stale again.
	g := reloadgate.New()
	g.Mutated()
	g.MarkLoaded(nav.TabLog)
	g.Mutated()

	if !g.ShouldAutoReload(nav.TabLog) {
		t.Error("TabLog should be stale after second mutation")
	}
}

func TestTableDriven(t *testing.T) {
	tests := []struct {
		name    string
		ops     func(g *reloadgate.ReloadGate)
		tab     nav.TabID
		want    bool
	}{
		{
			name: "no mutations ever",
			ops:  func(g *reloadgate.ReloadGate) {},
			tab:  nav.TabLog,
			want: false,
		},
		{
			name: "mutated, not loaded",
			ops: func(g *reloadgate.ReloadGate) {
				g.Mutated()
			},
			tab:  nav.TabLog,
			want: true,
		},
		{
			name: "mutated then loaded",
			ops: func(g *reloadgate.ReloadGate) {
				g.Mutated()
				g.MarkLoaded(nav.TabLog)
			},
			tab:  nav.TabLog,
			want: false,
		},
		{
			name: "loaded then mutated",
			ops: func(g *reloadgate.ReloadGate) {
				g.MarkLoaded(nav.TabLog)
				g.Mutated()
			},
			tab:  nav.TabLog,
			want: true,
		},
		{
			name: "mutated twice, loaded once after first",
			ops: func(g *reloadgate.ReloadGate) {
				g.Mutated()
				g.MarkLoaded(nav.TabLog)
				g.Mutated()
			},
			tab:  nav.TabLog,
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := reloadgate.New()
			tc.ops(g)
			got := g.ShouldAutoReload(tc.tab)
			if got != tc.want {
				t.Errorf("ShouldAutoReload(%q) = %v, want %v", tc.tab, got, tc.want)
			}
		})
	}
}
