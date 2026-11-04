package broker

import (
	"testing"

	"github.com/elentok/gx/server/wire"
)

func TestBroker_SlowSubscriberIsDroppedOthersKeepContiguousSeq(t *testing.T) {
	t.Parallel()
	b := New(2)
	slow, _, err := b.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	fast, cancel, err := b.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	var got []uint64
	for range 5 {
		b.Publish(wire.EventTicketChanged, "p:e/01")
		got = append(got, (<-fast).Seq) // keeps up
	}
	for i, seq := range got {
		if seq != uint64(i+1) {
			t.Fatalf("fast seqs = %v, want contiguous from 1", got)
		}
	}

	n := 0
	for range slow {
		n++
	}
	if n != 2 {
		t.Errorf("slow subscriber received %d events before the drop, want its buffer (2)", n)
	}
}

func TestBroker_SubscribeReplaysFromSeqAndRefusesUnknownSeq(t *testing.T) {
	t.Parallel()
	b := New(0)
	for range 3 {
		b.Publish(wire.EventTicketChanged, "p:e/01")
	}

	ch, cancel, err := b.Subscribe(1)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if first := <-ch; first.Seq != 2 {
		t.Errorf("first replayed seq = %d, want 2", first.Seq)
	}
	if _, _, err := b.Subscribe(99); err == nil {
		t.Error("subscribing from a future seq succeeded")
	}
}
