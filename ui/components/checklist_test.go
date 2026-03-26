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
