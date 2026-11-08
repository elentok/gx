package sched

import (
	"testing"
	"time"
)

// A human Retry and the claim loop reserve through claimAndReserve, so one
// cannot count a slot as free while the other is between its cap check and its
// reservation.
func TestClaimAndReserve_SerialisesCallers(t *testing.T) {
	t.Parallel()
	s := &Scheduler{}
	inside, release, second := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		_, _ = s.claimAndReserve(func() (*claimed, error) {
			close(inside)
			<-release
			return nil, nil
		})
	}()
	<-inside
	go func() {
		_, _ = s.claimAndReserve(func() (*claimed, error) {
			close(second)
			return nil, nil
		})
	}()
	select {
	case <-second:
		t.Fatal("a second claim ran while the first held the claim lock")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-second:
	case <-time.After(5 * time.Second):
		t.Fatal("the second claim never ran after the first finished")
	}
}
