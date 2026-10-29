package server

import (
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
	"time"
)

var webTLSNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func TestWebTLSConstraintDomains(t *testing.T) {
	tests := []struct {
		name  string
		hosts []string
		want  []string
	}{
		{"none", nil, []string{"localhost"}},
		{"parent domain", []string{"gertsel.elentok.com"}, []string{"localhost", "elentok.com"}},
		{"two hosts one parent", []string{"a.elentok.com", "b.elentok.com"}, []string{"localhost", "elentok.com"}},
		{"single label", []string{"mybox"}, []string{"localhost", "mybox"}},
		{"parent is public suffix", []string{"elentok.com"}, []string{"localhost", "elentok.com"}},
		{"private public suffix", []string{"gx.github.io"}, []string{"localhost", "gx.github.io"}},
		{"normalised and IP ignored", []string{"Foo.Example.org.", "10.0.0.1"}, []string{"localhost", "example.org"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := webConstraintDomains(tt.hosts); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebTLSRootFields(t *testing.T) {
	root, err := newWebRoot([]string{"gertsel.elentok.com"}, webTLSNow)
	if err != nil {
		t.Fatal(err)
	}
	c := root.Cert
	if !c.IsCA || !c.BasicConstraintsValid || c.MaxPathLen != 0 || !c.MaxPathLenZero {
		t.Errorf("CA constraints wrong: %+v", c)
	}
	if c.KeyUsage != x509.KeyUsageCertSign|x509.KeyUsageCRLSign {
		t.Errorf("key usage = %v", c.KeyUsage)
	}
	if !c.PermittedDNSDomainsCritical {
		t.Error("name constraints not critical")
	}
	if !reflect.DeepEqual(c.PermittedDNSDomains, []string{"localhost", "elentok.com"}) {
		t.Errorf("dns constraints = %v", c.PermittedDNSDomains)
	}
	var ranges []string
	for _, n := range c.PermittedIPRanges {
		ranges = append(ranges, n.String())
	}
	wantRanges := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "::1/128", "fc00::/7", "fe80::/10"}
	if !reflect.DeepEqual(ranges, wantRanges) {
		t.Errorf("ip ranges = %v", ranges)
	}
	if got := c.NotAfter.Sub(webTLSNow); got != webRootValidity {
		t.Errorf("validity = %v", got)
	}
	if c.PublicKeyAlgorithm != x509.ECDSA || root.Key.Curve.Params().Name != "P-256" {
		t.Error("root key is not ECDSA P-256")
	}
	if c.Subject.String() != c.Issuer.String() {
		t.Error("root not self-signed")
	}
}

func TestWebTLSLeafFieldsAndVerify(t *testing.T) {
	root, err := newWebRoot([]string{"gertsel.elentok.com"}, webTLSNow)
	if err != nil {
		t.Fatal(err)
	}
	ifaces := []net.IP{net.ParseIP("192.168.1.5"), net.ParseIP("8.8.8.8"), net.ParseIP("2001:db8::1"), net.ParseIP("fd00::7")}
	leaf, skipped, err := newWebLeaf(root, []string{"gertsel.elentok.com", "example.com"}, ifaces, net.IPv4zero, webTLSNow)
	if err != nil {
		t.Fatal(err)
	}
	c := leaf.Cert
	if c.IsCA || c.KeyUsage != x509.KeyUsageDigitalSignature || !reflect.DeepEqual(c.ExtKeyUsage, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}) {
		t.Errorf("leaf usage wrong: %+v", c)
	}
	if !c.NotBefore.Equal(webTLSNow.Add(-time.Hour)) || !c.NotAfter.Equal(webTLSNow.Add(365*24*time.Hour)) {
		t.Errorf("validity = %v .. %v", c.NotBefore, c.NotAfter)
	}
	if !reflect.DeepEqual(c.DNSNames, []string{"localhost", "gertsel.elentok.com"}) {
		t.Errorf("dns names = %v", c.DNSNames)
	}
	var ips []string
	for _, ip := range c.IPAddresses {
		ips = append(ips, ip.String())
	}
	if !reflect.DeepEqual(ips, []string{"192.168.1.5", "fd00::7"}) {
		t.Errorf("ips = %v", ips)
	}
	if !reflect.DeepEqual(skipped.Hosts, []string{"example.com"}) {
		t.Errorf("skipped hosts = %v", skipped.Hosts)
	}
	var skippedIPs []string
	for _, ip := range skipped.IPs {
		skippedIPs = append(skippedIPs, ip.String())
	}
	if !reflect.DeepEqual(skippedIPs, []string{"8.8.8.8", "2001:db8::1", "0.0.0.0"}) {
		t.Errorf("skipped ips = %v", skippedIPs)
	}

	pool := x509.NewCertPool()
	pool.AddCert(root.Cert)
	for _, name := range []string{"localhost", "gertsel.elentok.com"} {
		if _, err := c.Verify(x509.VerifyOptions{Roots: pool, DNSName: name, CurrentTime: webTLSNow}); err != nil {
			t.Errorf("verify %s: %v", name, err)
		}
	}
	if _, err := c.Verify(x509.VerifyOptions{Roots: pool, DNSName: "192.168.1.5", CurrentTime: webTLSNow}); err != nil {
		t.Errorf("verify ip: %v", err)
	}
}

func TestWebTLSRootRefusesOutsideNames(t *testing.T) {
	root, err := newWebRoot([]string{"gertsel.elentok.com"}, webTLSNow)
	if err != nil {
		t.Fatal(err)
	}
	// Sign a leaf for a name the constraints forbid, bypassing newWebLeaf's filter.
	forged := *root
	forged.Cert = &x509.Certificate{}
	*forged.Cert = *root.Cert
	forged.Cert.PermittedDNSDomains = []string{"example.com"}
	leaf, _, err := newWebLeaf(&forged, []string{"example.com"}, nil, nil, webTLSNow)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(root.Cert)
	if _, err := leaf.Cert.Verify(x509.VerifyOptions{Roots: pool, DNSName: "example.com", CurrentTime: webTLSNow}); err == nil {
		t.Fatal("leaf for example.com verified against the constrained root")
	}
}

func TestWebTLSFilesPersist(t *testing.T) {
	dir := t.TempDir()
	root, err := loadOrMakeWebRoot(dir, []string{"gertsel.elentok.com"}, webTLSNow)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _, err := newWebLeaf(root, nil, nil, nil, webTLSNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveWebLeaf(dir, leaf); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"root.pem", "root.key", "leaf.pem", "leaf.key"} {
		info, err := os.Stat(filepath.Join(dir, "web-tls", f))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %v", f, info.Mode().Perm())
		}
	}
	again, err := loadOrMakeWebRoot(dir, []string{"other.example.org"}, webTLSNow.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !again.Cert.Equal(root.Cert) || !again.Key.Equal(root.Key) {
		t.Error("second load made a different root")
	}
	loaded, err := loadWebLeaf(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Cert.Equal(leaf.Cert) || !loaded.Key.Equal(leaf.Key) {
		t.Error("leaf did not survive a reload")
	}
	if _, err := loadWebLeaf(t.TempDir()); !os.IsNotExist(err) {
		t.Errorf("missing leaf error = %v", err)
	}
}

func TestWebTLSReissueDecision(t *testing.T) {
	root, err := newWebRoot([]string{"gertsel.elentok.com"}, webTLSNow)
	if err != nil {
		t.Fatal(err)
	}
	hosts := []string{"gertsel.elentok.com"}
	leaf, _, err := newWebLeaf(root, hosts, []net.IP{net.ParseIP("192.168.1.5")}, nil, webTLSNow)
	if err != nil {
		t.Fatal(err)
	}
	day := 24 * time.Hour
	expiry := leaf.Cert.NotAfter
	tests := []struct {
		name    string
		hosts   []string
		localIP net.IP
		now     time.Time
		want    bool
	}{
		{"31 days left", hosts, net.ParseIP("192.168.1.5"), expiry.Add(-31 * day), false},
		{"29 days left", hosts, net.ParseIP("192.168.1.5"), expiry.Add(-29 * day), true},
		{"missing permitted IP", hosts, net.ParseIP("192.168.1.9"), webTLSNow, true},
		{"unpermitted IP ignored", hosts, net.ParseIP("8.8.8.8"), webTLSNow, false},
		{"unspecified IP ignored", hosts, net.IPv4zero, webTLSNow, false},
		{"no local IP", hosts, nil, webTLSNow, false},
		{"missing permitted host", []string{"gertsel.elentok.com", "new.elentok.com"}, net.ParseIP("192.168.1.5"), webTLSNow, true},
		{"unpermitted host ignored", []string{"gertsel.elentok.com", "example.com"}, net.ParseIP("192.168.1.5"), webTLSNow, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := webLeafNeedsReissue(leaf.Cert, root.Cert, tt.hosts, tt.localIP, tt.now); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebTLSFingerprintFormat(t *testing.T) {
	fp := webRootFingerprint([]byte("root der"))
	if !regexp.MustCompile(`^([0-9a-f]{8} ){7}[0-9a-f]{8}$`).MatchString(fp) {
		t.Fatalf("fingerprint = %q", fp)
	}
	if fp != webRootFingerprint([]byte("root der")) || fp == webRootFingerprint([]byte("other")) {
		t.Fatal("fingerprint not a stable function of the DER")
	}
}
