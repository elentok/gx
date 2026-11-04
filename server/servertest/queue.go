package servertest

import (
	"context"
	"testing"
)

// QueueAddresses returns the addresses of the queue's items, in order.
func QueueAddresses(t *testing.T, h *Harness) []string {
	t.Helper()
	items, err := h.Client.QueueItems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, it := range items {
		got = append(got, it.Address)
	}
	return got
}
