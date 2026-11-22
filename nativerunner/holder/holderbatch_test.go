package holder_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner/holder"
)

func begin(t *testing.T, c *holder.HolderClient) {
	t.Helper()
	if err := c.BatchBegin(); err != nil {
		t.Fatal(err)
	}
}

func end(t *testing.T, c *holder.HolderClient) {
	t.Helper()
	if err := c.BatchEnd(); err != nil {
		t.Fatal(err)
	}
}

// wantOwner reads the next owner report on a.
func wantOwner(t *testing.T, a *holder.HolderAttach, want holder.AttachOwner) {
	t.Helper()
	if o := waitMsg(t, a, holder.StreamOwner).Owner; o != want {
		t.Fatalf("owner report = %+v; want %+v", o, want)
	}
}

func wantBlocked(t *testing.T, a *holder.HolderAttach, r rune) {
	t.Helper()
	typeKey(t, a, r)
	waitMsg(t, a, holder.StreamBlocked)
}

func TestBatch_TakesAndReturnsTheInput(t *testing.T) {
	c := ownerHolder(t)
	a, _ := attachOwned(t, c)
	resize(t, a, 100, 30)
	waitPTY(t, c, [2]int{100, 30})

	begin(t, c)
	wantOwner(t, a, holder.AttachOwner{GX: true})
	wantBlocked(t, a, 'x')
	if err := c.SendKeys("g"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "gx's key", func() bool { return screenHas(c, "g") })

	end(t, c)
	wantOwner(t, a, holder.AttachOwner{Self: true})
	typeKey(t, a, 'y')
	waitFor(t, "the owner's key after the batch", func() bool { return screenHas(c, "y") })
	if screenHas(c, "x") {
		t.Error("a key typed during the batch reached the child")
	}
}

func TestBatch_BatchHelperAlwaysEnds(t *testing.T) {
	c := ownerHolder(t)
	a, _ := attachOwned(t, c)
	_ = c.Batch(func() error { return holder.ErrHolderDenyBusy })
	wantOwner(t, a, holder.AttachOwner{GX: true})
	wantOwner(t, a, holder.AttachOwner{Self: true})
}

func TestBatch_TakeOverDuringABatchIsIgnored(t *testing.T) {
	c := ownerHolder(t)
	a, _ := attachOwned(t, c)
	b, _ := attachOwned(t, c)
	resize(t, a, 100, 30)
	resize(t, b, 60, 20)
	waitPTY(t, c, [2]int{100, 30})

	begin(t, c)
	wantOwner(t, b, holder.AttachOwner{GX: true})
	takeOver(t, b)
	wantBlocked(t, b, 'x')
	if _, st, _ := c.StyledScreenRead(); st.PTY != [2]int{100, 30} {
		t.Errorf("pty = %v after a take-over in a batch; want 100x30", st.PTY)
	}

	end(t, c)
	wantOwner(t, a, holder.AttachOwner{GX: true})
	wantOwner(t, a, holder.AttachOwner{Self: true})
	wantOwner(t, b, holder.AttachOwner{})
}

func TestBatch_PreviousOwnerDetachedLeavesGxOwning(t *testing.T) {
	c := ownerHolder(t)
	a, _ := attachOwned(t, c)
	b, _ := attachOwned(t, c)
	begin(t, c)
	wantOwner(t, b, holder.AttachOwner{GX: true})
	_ = a.Close()
	end(t, c)
	// Nothing to report: gx still owns, and b stays blocked.
	wantBlocked(t, b, 'x')
	if screenHas(c, "x") {
		t.Error("a non-owner's key reached the child")
	}
}

func TestBatch_ClientAttachedDuringABatchOwnsAfterItAndSizesThen(t *testing.T) {
	c := ownerHolder(t)
	begin(t, c)
	a, owner := attachOwned(t, c)
	if !owner.GX {
		t.Fatalf("attach during a batch owner report = %+v; want GX", owner)
	}
	resize(t, a, 100, 30)
	wantBlocked(t, a, 'x')
	if _, st, _ := c.StyledScreenRead(); st.PTY != [2]int{holder.HolderCols, holder.HolderRows} {
		t.Fatalf("pty = %v during the batch; want the spawn size", st.PTY)
	}

	end(t, c)
	wantOwner(t, a, holder.AttachOwner{Self: true})
	waitPTY(t, c, [2]int{100, 30})
}

func TestBatch_AbnormalEndReleasesAfterTheTimeout(t *testing.T) {
	holder.SetBatchTimeout(t, 100*time.Millisecond)
	c := ownerHolder(t)
	a, _ := attachOwned(t, c)
	begin(t, c)
	wantOwner(t, a, holder.AttachOwner{GX: true})
	wantOwner(t, a, holder.AttachOwner{Self: true})
}

func TestBatch_GxKeysRestartTheTimeout(t *testing.T) {
	holder.SetBatchTimeout(t, 400*time.Millisecond)
	c := ownerHolder(t)
	a, _ := attachOwned(t, c)
	start := time.Now()
	begin(t, c)
	wantOwner(t, a, holder.AttachOwner{GX: true})
	last := start
	for range 3 {
		waitFor(t, "the gap between gx's keys", func() bool { return time.Since(last) >= 200*time.Millisecond })
		if err := c.SendKeys("k"); err != nil {
			t.Fatal(err)
		}
		last = time.Now()
	}
	wantOwner(t, a, holder.AttachOwner{Self: true})
	if got := time.Since(start); got < 900*time.Millisecond {
		t.Errorf("released after %v; want it counted from gx's last key", got)
	}
}

func TestBatch_NoBatchOpenMeansNoHold(t *testing.T) {
	c := ownerHolder(t)
	a, _ := attachOwned(t, c)
	begin(t, c)
	end(t, c)
	typeKey(t, a, 'y')
	waitFor(t, "the owner's key", func() bool { return screenHas(c, "y") })
}

func TestBatch_LaunchHoldBlocksAnEarlyClientUntilTheFirstBatchEnds(t *testing.T) {
	dir := shortTempDir(t)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_ = holder.RunTestHolderWith(dir, settableHarness{w: &settableWatcher{}}, holder.HolderOptions{HoldInput: true},
			[]string{"sh", "-c", `printf ready; cat >/dev/null`})
	}()
	var c *holder.HolderClient
	waitFor(t, "holder.sock answers", func() bool {
		var err error
		c, err = holder.DialHolder(dir)
		return err == nil
	})
	t.Cleanup(func() {
		_ = c.Stop()
		<-finished
	})
	waitFor(t, "child ready", func() bool { return screenHas(c, "ready") })

	a, owner := attachOwned(t, c)
	if !owner.GX {
		t.Fatalf("early attach owner report = %+v; want GX", owner)
	}
	resize(t, a, 100, 30)
	takeOver(t, a)
	wantBlocked(t, a, 'x')
	if _, st, _ := c.StyledScreenRead(); st.PTY != [2]int{holder.HolderCols, holder.HolderRows} {
		t.Fatalf("pty = %v during the launch hold; want the spawn size", st.PTY)
	}

	begin(t, c)
	end(t, c)
	wantOwner(t, a, holder.AttachOwner{Self: true})
	waitPTY(t, c, [2]int{100, 30})
	typeKey(t, a, 'y')
	waitFor(t, "the owner's key", func() bool { return screenHas(c, "y") })
	if screenHas(c, "x") {
		t.Error("a key typed during the launch hold reached the child")
	}
}
