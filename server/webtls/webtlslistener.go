package webtls

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

// tlsRecordHandshake is the first byte of a TLS ClientHello record.
const tlsRecordHandshake = 0x16

// webSniffTimeout bounds how long a silent connection may hold its first byte back.
const webSniffTimeout = 10 * time.Second

// Certs serves the web leaf and re-issues it from the same root, so no
// device re-trusts anything. It re-issues at start, on an allowed-hosts reload,
// and in GetCertificate when a connection needs a name the leaf lacks or the
// leaf is close to expiry. Nothing polls and no timer runs. The exported
// fields are the settings NewCerts needs.
type Certs struct {
	StateDir string
	ListenIP net.IP
	Now      func() time.Time
	Hosts    func() []string
	IfaceIPs func() []net.IP
	Log      *slog.Logger

	root    *webTLSKey
	mu      sync.Mutex
	leaf    *webTLSKey
	cert    *tls.Certificate
	skipped LeafSkipped
}

// NewCerts loads the saved leaf, or issues one when it is missing or no
// longer fits the root, the allowed hosts and the listen address.
func NewCerts(c *Certs) (*Certs, error) {
	now := c.Now()
	root, err := loadOrMakeWebRoot(c.StateDir, c.Hosts(), now)
	if err != nil {
		return nil, err
	}
	c.root = root
	leaf, err := loadWebLeaf(c.StateDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if leaf == nil || webLeafNeedsReissue(leaf.Cert, root.Cert, c.Hosts(), c.ListenIP, now) {
		return c, c.reissueLocked(nil)
	}
	c.setLeaf(leaf)
	return c, nil
}

func (c *Certs) setLeaf(leaf *webTLSKey) {
	c.leaf = leaf
	c.cert = &tls.Certificate{Certificate: [][]byte{leaf.DER}, PrivateKey: leaf.Key}
}

// reissueLocked signs a new leaf for the current hosts and interfaces, plus
// extraIP (the local IP of the connection that asked). Callers hold c.mu, or
// own c while it is being built.
func (c *Certs) reissueLocked(extraIP net.IP) error {
	ips := c.IfaceIPs()
	if extraIP != nil {
		ips = append(append([]net.IP{}, ips...), extraIP)
	}
	leaf, skipped, err := newWebLeaf(c.root, c.Hosts(), ips, c.ListenIP, c.Now())
	if err != nil {
		return err
	}
	if err := saveWebLeaf(c.StateDir, leaf); err != nil {
		return err
	}
	c.setLeaf(leaf)
	c.skipped = skipped
	if len(skipped.Hosts) > 0 || len(skipped.IPs) > 0 {
		ipStrs := make([]string, len(skipped.IPs))
		for i, ip := range skipped.IPs {
			ipStrs[i] = ip.String()
		}
		level := slog.LevelInfo
		if len(skipped.Hosts) > 0 {
			level = slog.LevelWarn
		}
		c.Log.Log(context.Background(), level, "web certificate left out names the root does not permit", "hosts", skipped.Hosts, "ips", ipStrs)
	}
	return nil
}

// Reissue replaces the leaf unconditionally, for an allowed-hosts reload.
func (c *Certs) Reissue() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reissueLocked(nil)
}

// NewRoot replaces the root with one made from the current allowed hosts and
// signs a new leaf from it. A failure leaves the old root and leaf serving.
func (c *Certs) NewRoot() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	root, err := newWebRoot(c.Hosts(), c.Now())
	if err != nil {
		return err
	}
	old := c.root
	dir := webTLSDir(c.StateDir)
	if err := writeWebTLSKey(dir, "root", root); err != nil {
		return err
	}
	c.root = root
	if err := c.reissueLocked(nil); err != nil {
		c.root = old
		if rerr := writeWebTLSKey(dir, "root", old); rerr != nil {
			c.Log.Error("web root rollback failed", "err", rerr)
		}
		return err
	}
	return nil
}

// Root is the certificate the profile and .cer routes serve.
func (c *Certs) Root() *x509.Certificate {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.root.Cert
}

// Skipped is what the last leaf left out because the root does not permit it.
func (c *Certs) Skipped() LeafSkipped {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.skipped
}

// GetCertificate is the TLS config's certificate source. A failed re-issue
// keeps serving the current leaf, which is still valid.
func (c *Certs) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	var localIP net.IP
	if hello.Conn != nil {
		if a, ok := hello.Conn.LocalAddr().(*net.TCPAddr); ok {
			localIP = a.IP
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if webLeafNeedsReissue(c.leaf.Cert, c.root.Cert, c.Hosts(), localIP, c.Now()) {
		if err := c.reissueLocked(localIP); err != nil {
			c.Log.Error("web certificate re-issue failed", "err", err)
		}
	}
	return c.cert, nil
}

// InterfaceIPs lists the addresses of the machine's network interfaces.
func InterfaceIPs() []net.IP {
	addrs, _ := net.InterfaceAddrs()
	var ips []net.IP
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			ips = append(ips, n.IP)
		}
	}
	return ips
}

// sniffListener serves TLS and plain HTTP on one port. A connection whose first
// byte is not a TLS handshake stays plain, and the handler redirects it.
type sniffListener struct {
	net.Listener
	cfg   *tls.Config
	conns chan net.Conn
	done  chan struct{}
}

func NewSniffListener(ln net.Listener, cfg *tls.Config) *sniffListener {
	l := &sniffListener{Listener: ln, cfg: cfg, conns: make(chan net.Conn), done: make(chan struct{})}
	go l.acceptLoop()
	return l
}

func (l *sniffListener) acceptLoop() {
	defer close(l.done)
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return
		}
		go l.sniff(c)
	}
}

func (l *sniffListener) sniff(c net.Conn) {
	_ = c.SetReadDeadline(time.Now().Add(webSniffTimeout))
	br := bufio.NewReader(c)
	first, err := br.Peek(1)
	if err != nil {
		c.Close()
		return
	}
	_ = c.SetReadDeadline(time.Time{})
	var out net.Conn = &bufferedConn{Conn: c, r: br}
	if first[0] == tlsRecordHandshake {
		out = tls.Server(out, l.cfg)
	}
	select {
	case l.conns <- out:
	case <-l.done:
		out.Close()
	}
}

func (l *sniffListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

// bufferedConn reads through the reader that already holds the sniffed byte.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// RedirectPlainHTTP answers a request that did not arrive over TLS with a
// redirect to the same host and path over https. Run it behind RequireHost, so
// only an allowed Host gets a redirect.
func RedirectPlainHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil {
			http.Redirect(w, r, "https://"+r.Host+r.URL.RequestURI(), http.StatusPermanentRedirect)
			return
		}
		next.ServeHTTP(w, r)
	})
}
