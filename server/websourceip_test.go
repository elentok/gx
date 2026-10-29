package server

import (
	"io"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWebSourceIP_Rules(t *testing.T) {
	cases := map[string]bool{
		"10.1.2.3":        true,
		"172.16.0.1":      true,
		"172.31.255.255":  true,
		"172.32.0.1":      false,
		"192.168.1.1":     true,
		"100.64.0.1":      true,
		"100.128.0.1":     false,
		"127.0.0.1":       true,
		"169.254.1.1":     true,
		"::1":             true,
		"fe80::1":         true,
		"fd12:3456::1":    true,
		"8.8.8.8":         false,
		"::ffff:10.0.0.5": true,
		"::ffff:8.8.8.8":  false,
		"2001:4860::8888": false,
	}
	for s, want := range cases {
		if got := webSourceAllowed(netip.MustParseAddr(s)); got != want {
			t.Errorf("%s: allowed = %v, want %v", s, got, want)
		}
	}
}

type fakeConn struct {
	net.Conn
	remote net.Addr
	closed bool
}

func (c *fakeConn) RemoteAddr() net.Addr { return c.remote }
func (c *fakeConn) Close() error         { c.closed = true; return nil }

type fakeListener struct {
	net.Listener
	conns chan net.Conn
}

func (l *fakeListener) Accept() (net.Conn, error) {
	c, ok := <-l.conns
	if !ok {
		return nil, net.ErrClosed
	}
	return c, nil
}

type lockedBuf struct {
	mu sync.Mutex
	sb strings.Builder
}

func (b *lockedBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.Write(p)
}

func TestWebSourceIP_ListenerClosesPublicPeers(t *testing.T) {
	var logs lockedBuf
	now := time.Unix(1000, 0)
	fl := &fakeListener{conns: make(chan net.Conn, 8)}
	ln := newSourceIPListener(fl, slog.New(slog.NewTextHandler(&logs, nil)), func() time.Time { return now })

	pub := func(ip string) *fakeConn {
		return &fakeConn{remote: &net.TCPAddr{IP: net.ParseIP(ip), Port: 5000}}
	}
	// The forwarded header is only bytes on the conn; the rule never reads it.
	spoofed := &fakeConn{Conn: nopConn{strings.NewReader("GET / HTTP/1.1\r\nX-Forwarded-For: 10.0.0.1\r\n\r\n")}, remote: &net.TCPAddr{IP: net.ParseIP("8.8.8.8"), Port: 1}}
	second := pub("8.8.8.8")
	other := pub("1.1.1.1")
	later := pub("8.8.8.8")
	good := pub("10.0.0.7")
	for _, c := range []*fakeConn{spoofed, second, other} {
		fl.conns <- c
	}
	fl.conns <- good
	got, err := ln.Accept()
	if err != nil || got != net.Conn(good) {
		t.Fatalf("Accept = %v, %v; want the private conn", got, err)
	}
	for _, c := range []*fakeConn{spoofed, second, other} {
		if !c.closed {
			t.Errorf("%v not closed", c.remote)
		}
	}
	if good.closed {
		t.Error("allowed conn was closed")
	}
	if n := strings.Count(logs.sb.String(), "ip=8.8.8.8"); n != 1 {
		t.Errorf("8.8.8.8 logged %d times in a minute, want 1:\n%s", n, logs.sb.String())
	}
	if !strings.Contains(logs.sb.String(), "ip=1.1.1.1") {
		t.Errorf("1.1.1.1 not logged:\n%s", logs.sb.String())
	}

	now = now.Add(time.Minute)
	fl.conns <- later
	fl.conns <- pub("10.0.0.8")
	if _, err := ln.Accept(); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(logs.sb.String(), "ip=8.8.8.8"); n != 2 {
		t.Errorf("8.8.8.8 logged %d times after a minute, want 2", n)
	}
}

type nopConn struct{ io.Reader }

func (nopConn) Close() error                     { return nil }
func (nopConn) LocalAddr() net.Addr              { return nil }
func (nopConn) RemoteAddr() net.Addr             { return nil }
func (nopConn) Write(p []byte) (int, error)      { return len(p), nil }
func (nopConn) SetDeadline(time.Time) error      { return nil }
func (nopConn) SetReadDeadline(time.Time) error  { return nil }
func (nopConn) SetWriteDeadline(time.Time) error { return nil }
