package server

import (
	"bytes"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"
)

type fakeLocalConn struct {
	net.Conn
	local net.Addr
}

func (c fakeLocalConn) LocalAddr() net.Addr { return c.local }

func newTestWebCerts(t *testing.T, dir string, now time.Time, hosts []string, log *slog.Logger) *webCerts {
	t.Helper()
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	c, err := newWebCerts(&webCerts{
		stateDir: dir,
		listenIP: net.ParseIP("127.0.0.1"),
		now:      func() time.Time { return now },
		hosts:    func() []string { return hosts },
		ifaceIPs: func() []net.IP { return []net.IP{net.ParseIP("127.0.0.1")} },
		log:      log,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestWebLeaf_ReissuedAtStartUnder30DaysLeft(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	first := newTestWebCerts(t, dir, webTLSNow, nil, nil)

	same := newTestWebCerts(t, dir, webTLSNow.Add(24*time.Hour), nil, nil)
	if same.leaf.Cert.SerialNumber.Cmp(first.leaf.Cert.SerialNumber) != 0 {
		t.Fatal("a fresh leaf was re-issued at start")
	}

	late := newTestWebCerts(t, dir, first.leaf.Cert.NotAfter.Add(-29*24*time.Hour), nil, nil)
	if late.leaf.Cert.SerialNumber.Cmp(first.leaf.Cert.SerialNumber) == 0 {
		t.Fatal("leaf with 29 days left was kept at start")
	}
	if !bytes.Equal(late.root.DER, first.root.DER) {
		t.Error("root changed on re-issue")
	}
	if err := late.leaf.Cert.CheckSignatureFrom(first.root.Cert); err != nil {
		t.Errorf("new leaf not signed by the old root: %v", err)
	}
}

func TestWebLeaf_ConnectionOnNewLocalIPGetsALeafWithIt(t *testing.T) {
	t.Parallel()
	c := newTestWebCerts(t, t.TempDir(), webTLSNow, nil, nil)
	hello := func(ip string) *tls.ClientHelloInfo {
		return &tls.ClientHelloInfo{Conn: fakeLocalConn{local: &net.TCPAddr{IP: net.ParseIP(ip), Port: 1}}}
	}
	leafHas := func(ip string) bool {
		for _, a := range c.leaf.Cert.IPAddresses {
			if a.Equal(net.ParseIP(ip)) {
				return true
			}
		}
		return false
	}
	if leafHas("127.0.0.2") {
		t.Fatal("leaf already has 127.0.0.2")
	}
	rootDER := c.root.DER
	cert, err := c.getCertificate(hello("127.0.0.2"))
	if err != nil {
		t.Fatal(err)
	}
	if !leafHas("127.0.0.2") || !bytes.Equal(cert.Certificate[0], c.leaf.DER) {
		t.Error("handshake on 127.0.0.2 did not get a leaf with it")
	}
	if !bytes.Equal(c.root.DER, rootDER) {
		t.Error("root changed")
	}

	before := c.leaf.Cert.SerialNumber
	if _, err := c.getCertificate(hello("8.8.8.8")); err != nil {
		t.Fatal(err)
	}
	if c.leaf.Cert.SerialNumber.Cmp(before) != 0 {
		t.Error("a public local IP the root does not permit caused a re-issue")
	}
}

func TestWebLeaf_ClockNear29DaysReissuesOnNextHandshake(t *testing.T) {
	t.Parallel()
	now := webTLSNow
	c := newTestWebCerts(t, t.TempDir(), now, nil, nil)
	c.now = func() time.Time { return now }
	before := c.leaf.Cert.SerialNumber
	if _, err := c.getCertificate(&tls.ClientHelloInfo{}); err != nil || c.leaf.Cert.SerialNumber.Cmp(before) != 0 {
		t.Fatalf("early handshake re-issued (err %v)", err)
	}
	now = c.leaf.Cert.NotAfter.Add(-29 * 24 * time.Hour)
	if _, err := c.getCertificate(&tls.ClientHelloInfo{}); err != nil {
		t.Fatal(err)
	}
	if c.leaf.Cert.SerialNumber.Cmp(before) == 0 {
		t.Error("handshake 29 days before expiry kept the old leaf")
	}
}
