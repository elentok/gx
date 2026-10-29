package server

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"os"
	"time"
)

// tlsRecordHandshake is the first byte of a TLS ClientHello record.
const tlsRecordHandshake = 0x16

// webSniffTimeout bounds how long a silent connection may hold its first byte back.
const webSniffTimeout = 10 * time.Second

// webServerCertificate loads the saved leaf, or issues one when it is missing or
// no longer fits the root, the allowed hosts and the listen address. It issues
// at start only.
func webServerCertificate(stateDir string, hosts []string, listenIP net.IP, ifaceIPs []net.IP, now time.Time) (tls.Certificate, *x509.Certificate, error) {
	root, err := loadOrMakeWebRoot(stateDir, hosts, now)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	leaf, err := loadWebLeaf(stateDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return tls.Certificate{}, nil, err
	}
	if leaf == nil || webLeafNeedsReissue(leaf.Cert, root.Cert, hosts, listenIP, now) {
		leaf, _, err = newWebLeaf(root, hosts, ifaceIPs, listenIP, now)
		if err != nil {
			return tls.Certificate{}, nil, err
		}
		if err := saveWebLeaf(stateDir, leaf); err != nil {
			return tls.Certificate{}, nil, err
		}
	}
	return tls.Certificate{Certificate: [][]byte{leaf.DER}, PrivateKey: leaf.Key}, root.Cert, nil
}

func interfaceIPs() []net.IP {
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

func newSniffListener(ln net.Listener, cfg *tls.Config) *sniffListener {
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

// redirectPlainHTTP answers a request that did not arrive over TLS with a
// redirect to the same host and path over https. Run it behind requireHost, so
// only an allowed Host gets a redirect.
func redirectPlainHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil {
			http.Redirect(w, r, "https://"+r.Host+r.URL.RequestURI(), http.StatusPermanentRedirect)
			return
		}
		next.ServeHTTP(w, r)
	})
}
