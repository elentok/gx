package server

import "testing"

func TestBroker_SlowSubscriberIsDroppedOthersKeepContiguousSeq(t *testing.T) {
	b := newBroker(2)
	slow, _, err := b.subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	fast, cancel, err := b.subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	var got []uint64
	for range 5 {
		b.publish(EventTicketChanged, "p:e/01")
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
	b := newBroker(0)
	for range 3 {
		b.publish(EventTicketChanged, "p:e/01")
	}

	ch, cancel, err := b.subscribe(1)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if first := <-ch; first.Seq != 2 {
		t.Errorf("first replayed seq = %d, want 2", first.Seq)
	}
	if _, _, err := b.subscribe(99); err == nil {
		t.Error("subscribing from a future seq succeeded")
	}
}
