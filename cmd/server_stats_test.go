package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
)

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

func TestServerStats_JSONHoldsGrillingSeries(t *testing.T) {
	s := server.Stats{Grilling: server.StatsGrilling{
		Words:     []server.StatsAmount{{Value: 12}},
		Questions: []server.StatsAmount{{NoData: true}},
	}}
	var buf bytes.Buffer
	if err := encodeProvenance(&buf, s, viaServer, callerActor(func() (string, error) { return "", nil })); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"grilling"`, `"words"`, `"questions"`} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("--json output lacks %s: %s", want, buf.String())
		}
	}
}
