package server_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// webLeaf dials the web listener and returns the leaf it serves.
func webLeaf(t *testing.T, h *servertest.Harness) *x509.Certificate {
	t.Helper()
	u, err := url.Parse(h.Server.WebURL())
	if err != nil {
		t.Fatal(err)
	}
	conn, err := tls.Dial("tcp", net.JoinHostPort("127.0.0.1", u.Port()), &tls.Config{InsecureSkipVerify: true, ServerName: "localhost"})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	return conn.ConnectionState().PeerCertificates[0]
}

func webRootPEM(t *testing.T, h *servertest.Harness) (*x509.Certificate, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(h.StateDir, "web-tls", "root.pem"))
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(data)
	root, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return root, data
}

func startLeafHarness(t *testing.T, hosts []string, mod func(*server.Config)) (*servertest.Harness, string, string) {
	t.Helper()
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path), func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.WebAllowedHosts = hosts
		if mod != nil {
			mod(c)
		}
	})
	return h, store, path
}

func writeHosts(t *testing.T, h *servertest.Harness, store, path string, hosts []string) {
	t.Helper()
	servertest.WriteConfig(t, path, store, map[string]any{
		"server": map[string]any{"web": map[string]any{"allowed-hosts": hosts}},
	})
	if res := reload(t, h); res.Refused {
		t.Fatalf("reload refused: %+v", res)
	}
}

func TestWebLeaf_ReloadAddingAHostServesALeafWithItFromTheSameRoot(t *testing.T) {
	t.Parallel()
	h, store, path := startLeafHarness(t, []string{"a.example.org"}, nil)
	before := webLeaf(t, h)
	root, rootPEM := webRootPEM(t, h)
	if slices.Contains(before.DNSNames, "b.example.org") {
		t.Fatal("leaf has b.example.org before the reload")
	}

	writeHosts(t, h, store, path, []string{"a.example.org", "b.example.org"})

	after := webLeaf(t, h)
	if !slices.Contains(after.DNSNames, "b.example.org") {
		t.Errorf("leaf names = %v, want b.example.org", after.DNSNames)
	}
	if err := after.CheckSignatureFrom(root); err != nil {
		t.Errorf("new leaf not signed by the root: %v", err)
	}
	if _, again := webRootPEM(t, h); string(again) != string(rootPEM) {
		t.Error("root changed on re-issue")
	}
}

func TestWebLeaf_HostOutsideConstraintsIsLeftOutAndWarned(t *testing.T) {
	t.Parallel()
	h, store, path := startLeafHarness(t, []string{"a.example.org"}, nil)

	writeHosts(t, h, store, path, []string{"a.example.org", "outside.example.net"})

	if leaf := webLeaf(t, h); slices.Contains(leaf.DNSNames, "outside.example.net") {
		t.Errorf("leaf names = %v, want outside.example.net left out", leaf.DNSNames)
	}
	log, err := os.ReadFile(server.LogPath(h.StateDir))
	if err != nil {
		t.Fatal(err)
	}
	var warned int
	for _, line := range strings.Split(string(log), "\n") {
		if strings.Contains(line, "WARN") && strings.Contains(line, "outside.example.net") {
			warned++
		}
	}
	if warned != 1 {
		t.Errorf("warning lines naming outside.example.net = %d, want 1\n%s", warned, log)
	}
}

func TestWebLeaf_ClockMovedNearExpiryReissuesOnTheNextHandshake(t *testing.T) {
	t.Parallel()
	var offset atomic.Int64
	h, _, _ := startLeafHarness(t, nil, func(c *server.Config) {
		c.Clock = func() time.Time { return time.Now().Add(time.Duration(offset.Load())) }
	})
	before := webLeaf(t, h)
	root, _ := webRootPEM(t, h)
	if again := webLeaf(t, h); again.SerialNumber.Cmp(before.SerialNumber) != 0 {
		t.Fatal("a fresh leaf was re-issued")
	}

	offset.Store(int64(before.NotAfter.Sub(time.Now()) - 29*24*time.Hour))

	after := webLeaf(t, h)
	if after.SerialNumber.Cmp(before.SerialNumber) == 0 {
		t.Fatal("handshake 29 days before expiry served the old leaf")
	}
	if err := after.CheckSignatureFrom(root); err != nil {
		t.Errorf("new leaf not signed by the root: %v", err)
	}
}
