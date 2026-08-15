package tree

import (
	"errors"
	"testing"
)

func TestLazySection_ExpandReturnsCommandOnceUntilInvalidated(t *testing.T) {
	t.Parallel()
	calls := 0
	sec := NewLazySection(3, func() ([]string, error) {
		calls++
		return []string{"a", "b", "c"}, nil
	})

	if cmd := sec.Expand(); cmd == nil {
		t.Fatal("expected a load command on first Expand")
	}
	if cmd := sec.Expand(); cmd != nil {
		t.Fatal("expected Expand to no-op while already loading")
	}

	sec.Deliver(LazyResultMsg[string]{Rows: []string{"a", "b", "c"}})
	if cmd := sec.Expand(); cmd != nil {
		t.Fatal("expected Expand to no-op once loaded")
	}

	sec.Invalidate()
	if cmd := sec.Expand(); cmd == nil {
		t.Fatal("expected Expand to return a load command again after Invalidate")
	}
	_ = calls
}

func TestLazySection_DeliverSuccessPopulatesRows(t *testing.T) {
	t.Parallel()
	sec := NewLazySection(2, func() ([]string, error) { return nil, nil })
	sec.Expand()

	consumed := sec.Deliver(LazyResultMsg[string]{Rows: []string{"x", "y"}})
	if !consumed {
		t.Fatal("expected Deliver to report the message as consumed")
	}
	if got := sec.Rows(); len(got) != 2 || got[0] != "x" || got[1] != "y" {
		t.Fatalf("Rows() = %v, want [x y]", got)
	}
	if sec.State() != LazyLoaded {
		t.Fatalf("State() = %v, want LazyLoaded", sec.State())
	}
}

func TestLazySection_DeliverFailureSetsFailedState(t *testing.T) {
	t.Parallel()
	sec := NewLazySection(1, func() ([]string, error) { return nil, nil })
	sec.Expand()

	wantErr := errors.New("boom")
	consumed := sec.Deliver(LazyResultMsg[string]{Err: wantErr})
	if !consumed {
		t.Fatal("expected Deliver to report the message as consumed")
	}
	if sec.State() != LazyFailed {
		t.Fatalf("State() = %v, want LazyFailed", sec.State())
	}
	if sec.Err() != wantErr {
		t.Fatalf("Err() = %v, want %v", sec.Err(), wantErr)
	}
}

func TestLazySection_DeliverIgnoresUnrelatedMessage(t *testing.T) {
	t.Parallel()
	sec := NewLazySection(1, func() ([]string, error) { return nil, nil })
	sec.Expand()

	if sec.Deliver("not a result message") {
		t.Fatal("expected Deliver to report false for an unrelated message")
	}
	if sec.State() != LazyLoading {
		t.Fatalf("State() = %v, want unchanged LazyLoading", sec.State())
	}
}

func TestLazySection_SetCountUnchangedLeavesLoadedCacheIntact(t *testing.T) {
	t.Parallel()
	sec := NewLazySection(2, func() ([]string, error) { return nil, nil })
	sec.Expand()
	sec.Deliver(LazyResultMsg[string]{Rows: []string{"a", "b"}})

	sec.SetCount(2)
	if sec.State() != LazyLoaded {
		t.Fatalf("State() = %v, want LazyLoaded to survive an unchanged SetCount", sec.State())
	}
	if len(sec.Rows()) != 2 {
		t.Fatalf("Rows() = %v, want cache intact", sec.Rows())
	}
}

func TestLazySection_SetCountChangedInvalidatesLoadedCache(t *testing.T) {
	t.Parallel()
	sec := NewLazySection(2, func() ([]string, error) { return nil, nil })
	sec.Expand()
	sec.Deliver(LazyResultMsg[string]{Rows: []string{"a", "b"}})

	sec.SetCount(3)
	if sec.State() != LazyIdle {
		t.Fatalf("State() = %v, want LazyIdle after a count change invalidates the cache", sec.State())
	}
	if sec.Rows() != nil {
		t.Fatalf("Rows() = %v, want nil after invalidation", sec.Rows())
	}
	if cmd := sec.Expand(); cmd == nil {
		t.Fatal("expected Expand to return a load command again after a count-change invalidation")
	}
}

func TestLazySection_InvalidateForcesNextExpandToLoadAgain(t *testing.T) {
	t.Parallel()
	sec := NewLazySection(1, func() ([]string, error) { return []string{"a"}, nil })
	sec.Expand()
	sec.Deliver(LazyResultMsg[string]{Rows: []string{"a"}})

	sec.Invalidate()
	if sec.State() != LazyIdle {
		t.Fatalf("State() = %v, want LazyIdle after Invalidate", sec.State())
	}
	if cmd := sec.Expand(); cmd == nil {
		t.Fatal("expected Expand to return a load command again after Invalidate")
	}
}

func TestLazySection_ExpandRetriesAfterFailure(t *testing.T) {
	t.Parallel()
	sec := NewLazySection(1, func() ([]string, error) { return nil, nil })
	sec.Expand()
	sec.Deliver(LazyResultMsg[string]{Err: errors.New("boom")})

	if cmd := sec.Expand(); cmd == nil {
		t.Fatal("expected Expand to return a load command again after a failed load")
	}
}
