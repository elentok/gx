package holder_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/nativerunner/holder"
)

// ownerHolder runs a child that echoes what it is typed.
func ownerHolder(t *testing.T) *holder.HolderClient {
	t.Helper()
	c, _ := scriptHolder(t, `printf ready; cat >/dev/null`)
	return c
}

// attachOwned attaches and returns the owner report that comes with it.
func attachOwned(t *testing.T, c *holder.HolderClient) (*holder.HolderAttach, holder.AttachOwner) {
	t.Helper()
	a := attach(t, c)
	return a, waitMsg(t, a, holder.StreamOwner).Owner
}

// waitMsg reads until a message of type typ, skipping the rest.
func waitMsg(t *testing.T, a *holder.HolderAttach, typ holder.StreamType) holder.AttachMessage {
	t.Helper()
	for {
		m, err := a.Next()
		if err != nil {
			t.Fatalf("waiting for message type %d: %v", typ, err)
		}
		if m.Type == typ {
			return m
		}
	}
}

func waitPTY(t *testing.T, c *holder.HolderClient, want [2]int) {
	t.Helper()
	waitFor(t, "the pty at the wanted size", func() bool {
		_, st, err := c.StyledScreenRead()
		return err == nil && st.PTY == want
	})
}

func screenHas(c *holder.HolderClient, text string) bool {
	rows, _, err := c.Screen(0)
	return err == nil && strings.Contains(strings.Join(rows, "\n"), text)
}

func typeKey(t *testing.T, a *holder.HolderAttach, r rune) {
	t.Helper()
	if err := a.SendKey(screen.Key{Code: r, Text: string(r)}); err != nil {
		t.Fatal(err)
	}
}

func resize(t *testing.T, a *holder.HolderAttach, cols, rows int) {
	t.Helper()
	if err := a.Resize(cols, rows); err != nil {
		t.Fatal(err)
	}
}

func takeOver(t *testing.T, a *holder.HolderAttach) {
	t.Helper()
	if err := a.TakeOver(); err != nil {
		t.Fatal(err)
	}
}

func TestAttachOwner_LoneAttachOwnsAndSetsTheSize(t *testing.T) {
	c := ownerHolder(t)
	a, owner := attachOwned(t, c)
	if !owner.Self {
		t.Fatalf("lone attach owner report = %+v; want Self", owner)
	}
	resize(t, a, 100, 30)
	waitPTY(t, c, [2]int{100, 30})
}

func TestAttachOwner_SecondAttachIsNotTheOwnerAndItsKeyIsBlocked(t *testing.T) {
	c := ownerHolder(t)
	a, _ := attachOwned(t, c)
	b, owner := attachOwned(t, c)
	if owner.Self || owner.GX {
		t.Fatalf("second attach owner report = %+v; want another attach's", owner)
	}
	typeKey(t, b, 'x')
	waitMsg(t, b, holder.StreamBlocked)
	// Keys are handled in order, so once a's key shows, b's was dropped.
	typeKey(t, a, 'y')
	waitFor(t, "the owner's key", func() bool { return screenHas(c, "y") })
	if screenHas(c, "x") {
		t.Error("the non-owner's key reached the child")
	}
}

func TestAttachOwner_FocusAndTypingNeverMoveOwnership(t *testing.T) {
	c := ownerHolder(t)
	a, _ := attachOwned(t, c)
	b, _ := attachOwned(t, c)
	resize(t, a, 100, 30)
	waitPTY(t, c, [2]int{100, 30})
	resize(t, b, 60, 20)
	typeKey(t, b, 'x')
	waitMsg(t, b, holder.StreamBlocked)
	typeKey(t, a, 'y')
	waitFor(t, "the owner's key", func() bool { return screenHas(c, "y") })
	if screenHas(c, "x") {
		t.Error("the non-owner's key reached the child")
	}
	_, st, err := c.StyledScreenRead()
	if err != nil {
		t.Fatal(err)
	}
	if st.PTY != [2]int{100, 30} {
		t.Errorf("pty = %v after a non-owner resized; want 100x30", st.PTY)
	}
	if !slices.Contains(st.Attaches, [2]int{60, 20}) {
		t.Errorf("attach sizes = %v; want the non-owner's own 60x20 view kept", st.Attaches)
	}
}

func TestAttachOwner_TakeOverMovesKeysAndSize(t *testing.T) {
	c := ownerHolder(t)
	a, _ := attachOwned(t, c)
	b, _ := attachOwned(t, c)
	resize(t, a, 100, 30)
	resize(t, b, 60, 20)
	waitPTY(t, c, [2]int{100, 30})

	takeOver(t, b)
	waitPTY(t, c, [2]int{60, 20})
	if o := waitMsg(t, b, holder.StreamOwner).Owner; !o.Self {
		t.Errorf("new owner report = %+v; want Self", o)
	}
	if o := waitMsg(t, a, holder.StreamOwner).Owner; o.Self || o.GX {
		t.Errorf("old owner report = %+v; want another attach's", o)
	}
	typeKey(t, a, 'x')
	waitMsg(t, a, holder.StreamBlocked)
	typeKey(t, b, 'y')
	waitFor(t, "the new owner's key", func() bool { return screenHas(c, "y") })
	if screenHas(c, "x") {
		t.Error("the old owner's key reached the child")
	}
	// The new owner's later resize reaches the pty; the old owner's does not.
	resize(t, b, 70, 25)
	waitPTY(t, c, [2]int{70, 25})
	resize(t, a, 90, 40)
	typeKey(t, b, 'z')
	waitFor(t, "a key after the old owner's resize", func() bool { return screenHas(c, "z") })
	if _, st, _ := c.StyledScreenRead(); st.PTY != [2]int{70, 25} {
		t.Errorf("pty = %v; want the owner's 70x25", st.PTY)
	}
}

func TestAttachOwner_OwnerDetachingLeavesGxOwningAndKeepsTheSize(t *testing.T) {
	c := ownerHolder(t)
	a, _ := attachOwned(t, c)
	b, _ := attachOwned(t, c)
	resize(t, a, 100, 30)
	waitPTY(t, c, [2]int{100, 30})

	_ = a.Close()
	if o := waitMsg(t, b, holder.StreamOwner).Owner; !o.GX || o.Self {
		t.Errorf("owner report after the owner left = %+v; want GX", o)
	}
	typeKey(t, b, 'x')
	waitMsg(t, b, holder.StreamBlocked)
	resize(t, b, 60, 20)
	typeKey(t, b, 'x')
	waitMsg(t, b, holder.StreamBlocked)
	waitPTY(t, c, [2]int{100, 30})
}

func TestAttachOwner_NoAttachKeepsTheSpawnSizeThenTheLastOne(t *testing.T) {
	c := ownerHolder(t)
	waitPTY(t, c, [2]int{holder.HolderCols, holder.HolderRows})
	a, _ := attachOwned(t, c)
	resize(t, a, 100, 30)
	waitPTY(t, c, [2]int{100, 30})
	_ = a.Close()
	waitFor(t, "the holder to drop the closed attach", func() bool {
		_, st, err := c.StyledScreenRead()
		return err == nil && len(st.Attaches) == 0
	})
	b, owner := attachOwned(t, c)
	if !owner.Self {
		t.Fatalf("attach after the last one left = %+v; want Self", owner)
	}
	waitPTY(t, c, [2]int{100, 30})
	_ = b.Close()
}

func TestAttachOwner_ThreeClients(t *testing.T) {
	c := ownerHolder(t)
	a, _ := attachOwned(t, c)
	b, _ := attachOwned(t, c)
	d, _ := attachOwned(t, c)
	resize(t, a, 100, 30)
	resize(t, d, 50, 15)
	takeOver(t, d)
	waitPTY(t, c, [2]int{50, 15})
	if o := waitMsg(t, d, holder.StreamOwner).Owner; !o.Self {
		t.Fatalf("new owner report = %+v; want Self", o)
	}
	for _, blocked := range []*holder.HolderAttach{a, b} {
		typeKey(t, blocked, 'x')
		waitMsg(t, blocked, holder.StreamBlocked)
	}
	typeKey(t, d, 'y')
	waitFor(t, "the third client's key", func() bool { return screenHas(c, "y") })
	if screenHas(c, "x") {
		t.Error("a non-owner's key reached the child")
	}
	// The second client takes over from the third; the first stays blocked.
	takeOver(t, b)
	if o := waitMsg(t, d, holder.StreamOwner).Owner; o.Self || o.GX {
		t.Fatalf("displaced owner report = %+v; want another attach's", o)
	}
	typeKey(t, d, 'q')
	waitMsg(t, d, holder.StreamBlocked)
	if screenHas(c, "q") {
		t.Error("the displaced owner's key reached the child")
	}
}
