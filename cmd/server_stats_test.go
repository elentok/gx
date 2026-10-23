package cmd

import "testing"

func TestServerStats_HasJSONRangeAndProjectFlags(t *testing.T) {
	root := newRootCmd(deps{})
	c, _, err := root.Find([]string{"server", "stats"})
	if err != nil || c.RunE == nil {
		t.Fatalf("server stats = %v, err %v; want a runnable verb", c, err)
	}
	for _, flag := range []string{"json", "range", "project"} {
		if c.Flags().Lookup(flag) == nil {
			t.Errorf("server stats has no --%s", flag)
		}
	}
}
