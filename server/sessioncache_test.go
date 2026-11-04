package server

import (
	"testing"
	"time"
)

func TestRecordedCache(t *testing.T) {
	var c recordedCache
	now := time.Now()
	if _, ok := c.lookup("a", now); ok {
		t.Fatal("empty cache hit")
	}
	c.store("a", false, now)
	if found, ok := c.lookup("a", now.Add(time.Second)); !ok || found {
		t.Fatalf("fresh miss = %v,%v", found, ok)
	}
	if _, ok := c.lookup("a", now.Add(recordedMissTTL+time.Second)); ok {
		t.Fatal("stale miss still trusted")
	}
	c.store("b", true, now)
	if found, ok := c.lookup("b", now.Add(24*time.Hour)); !ok || !found {
		t.Fatalf("hit expired: %v,%v", found, ok)
	}
}
