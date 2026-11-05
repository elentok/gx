package rigs

import (
	"context"
	"github.com/elentok/gx/server/servertest"
	"testing"
)

// QueueAddresses returns the addresses of the queue's items, in order.
func QueueAddresses(t *testing.T, h *servertest.Harness) []string {
	t.Helper()
	items, err := h.Client.Queue.Items(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, it := range items {
		got = append(got, it.Address)
	}
	return got
}
