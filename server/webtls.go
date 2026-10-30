package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"
)

const (
	webTLSDirName     = "web-tls"
	webRootValidity   = 10 * 365 * 24 * time.Hour
	webLeafValidity   = 365 * 24 * time.Hour
	webLeafBackdate   = time.Hour
	webLeafRenewAhead = 30 * 24 * time.Hour
)

// webRootIPRanges are the private, loopback and link-local ranges the root may
// sign IPs for. They are fixed in the root's name constraints and are the
// source IP gate's allow list.
var webRootIPRanges = ipNets(webAllowedPrefixes)

func ipNets(prefixes []netip.Prefix) []*net.IPNet {
	nets := make([]*net.IPNet, len(prefixes))
	for i, p := range prefixes {
		addr := p.Addr()
		nets[i] = &net.IPNet{IP: addr.AsSlice(), Mask: net.CIDRMask(p.Bits(), addr.BitLen())}
	}
	return nets
}

func mustParseCIDRs(cidrs ...string) []*net.IPNet {
	nets := make([]*net.IPNet, len(cidrs))
	for i, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		nets[i] = n
	}
	return nets
}

// webTLSKey is a certificate with its private key and DER encoding.
type webTLSKey struct {
	Cert *x509.Certificate
	DER  []byte
	Key  *ecdsa.PrivateKey
}

// webLeafSkipped lists what a leaf left out because the root does not permit it.
type webLeafSkipped struct {
	Hosts []string
	IPs   []net.IP
}

// webConstraintDomain is the name-constraint entry for one allowed host: the
// parent domain, or the host itself when it has a single label or its parent
// is a public suffix (a constraint on "com" would permit every .com name).
func webConstraintDomain(host string) string {
	host = normalizeHost(host)
	_, parent, ok := strings.Cut(host, ".")
	if !ok || parent == "" {
		return host
	}
	if suffix, _ := publicsuffix.PublicSuffix(parent); suffix == parent {
		return host
	}
	return parent
}

func webConstraintDomains(hosts []string) []string {
	domains := []string{"localhost"}
	seen := map[string]bool{"localhost": true}
	for _, h := range hosts {
		h = normalizeHost(h)
		if h == "" || net.ParseIP(h) != nil {
			continue
		}
		if d := webConstraintDomain(h); !seen[d] {
			seen[d] = true
			domains = append(domains, d)
		}
	}
	return domains
}

func webHostPermitted(root *x509.Certificate, host string) bool {
	for _, d := range root.PermittedDNSDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func webIPPermitted(root *x509.Certificate, ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() {
		return false
	}
	for _, n := range root.PermittedIPRanges {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func newWebTLSSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
}

func newWebRoot(hosts []string, now time.Time) (*webTLSKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := newWebTLSSerial()
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:                serial,
		Subject:                     pkix.Name{CommonName: "gx web root"},
		NotBefore:                   now.Add(-webLeafBackdate),
		NotAfter:                    now.Add(webRootValidity),
		KeyUsage:                    x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid:       true,
		IsCA:                        true,
		MaxPathLen:                  0,
		MaxPathLenZero:              true,
		PermittedDNSDomainsCritical: true,
		PermittedDNSDomains:         webConstraintDomains(hosts),
		PermittedIPRanges:           webRootIPRanges,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &webTLSKey{Cert: cert, DER: der, Key: key}, nil
}

// newWebLeaf signs a server certificate for localhost, the allowed hosts the
// root permits, and the interface and listen IPs inside the root's ranges.
// Unspecified addresses are never included. Whatever it leaves out comes back
// in the second result.
func newWebLeaf(root *webTLSKey, hosts []string, ifaceIPs []net.IP, listenIP net.IP, now time.Time) (*webTLSKey, webLeafSkipped, error) {
	var skipped webLeafSkipped
	names := []string{"localhost"}
	seenName := map[string]bool{"localhost": true}
	for _, h := range hosts {
		h = normalizeHost(h)
		if h == "" || seenName[h] || net.ParseIP(h) != nil {
			continue
		}
		seenName[h] = true
		if webHostPermitted(root.Cert, h) {
			names = append(names, h)
		} else {
			skipped.Hosts = append(skipped.Hosts, h)
		}
	}
	var ips []net.IP
	seenIP := map[string]bool{}
	for _, ip := range append(append([]net.IP{}, ifaceIPs...), listenIP) {
		if ip == nil || seenIP[ip.String()] {
			continue
		}
		seenIP[ip.String()] = true
		if webIPPermitted(root.Cert, ip) {
			ips = append(ips, ip)
		} else {
			skipped.IPs = append(skipped.IPs, ip)
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, skipped, err
	}
	serial, err := newWebTLSSerial()
	if err != nil {
		return nil, skipped, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "gx web server"},
		NotBefore:             now.Add(-webLeafBackdate),
		NotAfter:              now.Add(webLeafValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              names,
		IPAddresses:           ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, root.Cert, &key.PublicKey, root.Key)
	if err != nil {
		return nil, skipped, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, skipped, err
	}
	return &webTLSKey{Cert: cert, DER: der, Key: key}, skipped, nil
}

// webLeafNeedsReissue reports whether the leaf is close to expiry, or lacks a
// host or the connection's local IP that the root permits. Names the root
// cannot sign are ignored, since re-issuing would never add them.
func webLeafNeedsReissue(leaf, root *x509.Certificate, hosts []string, localIP net.IP, now time.Time) bool {
	if leaf.NotAfter.Sub(now) < webLeafRenewAhead {
		return true
	}
	if webIPPermitted(root, localIP) {
		has := false
		for _, ip := range leaf.IPAddresses {
			if ip.Equal(localIP) {
				has = true
				break
			}
		}
		if !has {
			return true
		}
	}
	for _, h := range hosts {
		h = normalizeHost(h)
		if h == "" || net.ParseIP(h) != nil || !webHostPermitted(root, h) {
			continue
		}
		has := false
		for _, n := range leaf.DNSNames {
			if n == h {
				has = true
				break
			}
		}
		if !has {
			return true
		}
	}
	return false
}

// webRootFingerprint is the SHA-256 of the root's DER as hex in 8 groups of 8.
func webRootFingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	h := hex.EncodeToString(sum[:])
	groups := make([]string, 0, 8)
	for i := 0; i < len(h); i += 8 {
		groups = append(groups, h[i:i+8])
	}
	return strings.Join(groups, " ")
}

func webTLSDir(stateDir string) string { return filepath.Join(stateDir, webTLSDirName) }

// loadOrMakeWebRoot loads the root from <stateDir>/web-tls, or makes and saves
// it when missing. The constraints are fixed at that moment.
func loadOrMakeWebRoot(stateDir string, hosts []string, now time.Time) (*webTLSKey, error) {
	dir := webTLSDir(stateDir)
	root, err := readWebTLSKey(dir, "root")
	if err == nil {
		return root, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	root, err = newWebRoot(hosts, now)
	if err != nil {
		return nil, err
	}
	return root, writeWebTLSKey(dir, "root", root)
}

// loadWebLeaf returns the saved leaf, or os.ErrNotExist when there is none.
func loadWebLeaf(stateDir string) (*webTLSKey, error) {
	return readWebTLSKey(webTLSDir(stateDir), "leaf")
}

func saveWebLeaf(stateDir string, leaf *webTLSKey) error {
	return writeWebTLSKey(webTLSDir(stateDir), "leaf", leaf)
}

func writeWebTLSKey(dir, name string, k *webTLSKey) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(k.Key)
	if err != nil {
		return err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: k.DER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	// The key goes first so a crash never leaves a certificate without its key.
	if err := writeFileAtomic(filepath.Join(dir, name+".key"), keyPEM); err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, name+".pem"), certPEM)
}

func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func readWebTLSKey(dir, name string) (*webTLSKey, error) {
	certPEM, err := os.ReadFile(filepath.Join(dir, name+".pem"))
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(filepath.Join(dir, name+".key"))
	if err != nil {
		return nil, err
	}
	certBlock, _ := pem.Decode(certPEM)
	keyBlock, _ := pem.Decode(keyPEM)
	if certBlock == nil || keyBlock == nil {
		return nil, fmt.Errorf("web-tls %s: not PEM", name)
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}
	return &webTLSKey{Cert: cert, DER: certBlock.Bytes, Key: key}, nil
}
