package servertest

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// webRoots holds the web TLS root of every harness started in this process, so
// one client reaches any of their web listeners.
var webRoots = struct {
	sync.Mutex
	pool *x509.CertPool
}{pool: x509.NewCertPool()}

// trustWebRoot adds the root in <stateDir>/web-tls to the roots WebHTTPClient
// trusts. A server with no web listener has none, and is skipped.
func trustWebRoot(stateDir string) {
	data, err := os.ReadFile(filepath.Join(stateDir, "web-tls", "root.pem"))
	if err != nil {
		return
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return
	}
	webRoots.Lock()
	defer webRoots.Unlock()
	webRoots.pool.AddCert(cert)
}

// WebHTTPClient returns a client that trusts the web TLS root of every harness
// started so far. It verifies the chain and the host against those roots.
func WebHTTPClient() *http.Client {
	return &http.Client{Transport: WebTransport()}
}

// WebTransport is the transport behind WebHTTPClient, for tests that need their
// own client policy.
func WebTransport() *http.Transport {
	return &http.Transport{TLSClientConfig: &tls.Config{
		// Verification happens in VerifyConnection, against roots that grow as
		// harnesses start.
		InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			webRoots.Lock()
			roots := webRoots.pool.Clone()
			webRoots.Unlock()
			inter := x509.NewCertPool()
			for _, c := range cs.PeerCertificates[1:] {
				inter.AddCert(c)
			}
			_, err := cs.PeerCertificates[0].Verify(x509.VerifyOptions{Roots: roots, Intermediates: inter, DNSName: cs.ServerName})
			return err
		},
	}}
}
