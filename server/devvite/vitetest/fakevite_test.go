package vitetest_test

import (
	"testing"

	"github.com/elentok/gx/server/devvite/vitetest"
)

func TestFreePort_NeverRepeatsAcrossFakes(t *testing.T) {
	t.Parallel()
	a, b := vitetest.NewFakeVite(), vitetest.NewFakeVite()
	seen := map[int]bool{}
	for range 50 {
		for _, f := range []*vitetest.FakeVite{a, b} {
			port, err := f.FreePort()
			if err != nil {
				t.Fatal(err)
			}
			if seen[port] {
				t.Fatalf("port %d handed out twice", port)
			}
			seen[port] = true
		}
	}
}
