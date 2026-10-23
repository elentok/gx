package tickets

import "testing"

func TestBlockedByEpicRef(t *testing.T) {
	cases := []struct {
		ref, project, epic string
		ok                 bool
	}{
		{"other-epic", "p", "other-epic", true},
		{"q:other-epic", "q", "other-epic", true},
		{"03", "", "", false},
		{"other-epic/03", "", "", false},
	}
	for _, c := range cases {
		project, epic, ok := BlockedByEpicRef("p", c.ref)
		if project != c.project || epic != c.epic || ok != c.ok {
			t.Errorf("BlockedByEpicRef(%q) = %q, %q, %v; want %q, %q, %v", c.ref, project, epic, ok, c.project, c.epic, c.ok)
		}
	}
}
