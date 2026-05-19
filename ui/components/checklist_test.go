package components

import "testing"

func TestChecklistUpdate_TogglesCurrentItemOnSpace(t *testing.T) {
	checklist := NewChecklist([]Item{
		{Label: "first", Value: "first", Checked: true},
		{Label: "second", Value: "second", Checked: true},
	})

	checklist = checklist.Update("space")

	if checklist.Items[0].Checked {
		t.Fatalf("first item remained checked after space toggle")
	}
	if !checklist.Items[1].Checked {
		t.Fatalf("second item should remain checked")
	}
}

func TestChecklistUpdate_Navigation(t *testing.T) {
	c := NewChecklist([]Item{
		{Label: "a"}, {Label: "b"}, {Label: "c"},
	})
	c = c.Update("j")
	if c.Cursor != 1 {
		t.Fatalf("j: expected cursor=1, got %d", c.Cursor)
	}
	c = c.Update("k")
	if c.Cursor != 0 {
		t.Fatalf("k: expected cursor=0, got %d", c.Cursor)
	}
	// k at top — stays at 0
	c = c.Update("k")
	if c.Cursor != 0 {
		t.Fatalf("k at top: expected cursor=0, got %d", c.Cursor)
	}
	// j past bottom — stays at last
	c = c.Update("j")
	c = c.Update("j")
	c = c.Update("j")
	if c.Cursor != 2 {
		t.Fatalf("j past bottom: expected cursor=2, got %d", c.Cursor)
	}
}

func TestChecklistUpdate_ToggleAll(t *testing.T) {
	c := NewChecklist([]Item{
		{Label: "a", Checked: true},
		{Label: "b", Checked: true},
	})
	// all checked → 'a' unchecks all
	c = c.Update("a")
	for i, item := range c.Items {
		if item.Checked {
			t.Fatalf("item %d should be unchecked after 'a'", i)
		}
	}
	// none checked → 'a' checks all
	c = c.Update("a")
	for i, item := range c.Items {
		if !item.Checked {
			t.Fatalf("item %d should be checked after second 'a'", i)
		}
	}
}

func TestChecklistChecked_ReturnsCheckedValues(t *testing.T) {
	c := NewChecklist([]Item{
		{Label: "a", Value: "v1", Checked: true},
		{Label: "b", Value: "v2", Checked: false},
		{Label: "c", Value: "v3", Checked: true},
	})
	got := c.Checked()
	if len(got) != 2 || got[0] != "v1" || got[1] != "v3" {
		t.Fatalf("Checked() = %v, want [v1 v3]", got)
	}
}

func TestChecklistUpdate_IgnoresUnknownKeys(t *testing.T) {
	c := NewChecklist([]Item{{Label: "x", Checked: false}})
	c2 := c.Update("z")
	if c2.Cursor != 0 || c2.Items[0].Checked {
		t.Fatal("unknown key should be a no-op")
	}
}
