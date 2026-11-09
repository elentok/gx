package cleanup

import (
	"context"
	"testing"
)

func TestInvalidate_DropsASizeMeasuredBeforeIt(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	c := New(Deps{Size: func(context.Context, string) (uint64, error) {
		close(started)
		<-release
		return 42, nil
	}})

	if n, missing := c.sizeOf("/wt", nil); n != nil || len(missing) != 1 {
		t.Fatalf("first sizeOf = %v, %v; want queued", n, missing)
	}
	done := make(chan struct{})
	go func() {
		c.measure("proj", "/wt")
		close(done)
	}()
	<-started
	c.Invalidate()
	close(release)
	<-done

	if n, _ := c.sizeOf("/wt", nil); n != nil {
		t.Errorf("size cached after Invalidate = %d, want none", *n)
	}
}
