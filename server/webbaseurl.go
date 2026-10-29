package server

import (
	"crypto/x509"
	"net"
)

// webLANRanges are the ranges a printed URL may fall back to when nothing more
// specific is configured: private, tailnet and ULA. Loopback and link-local are
// left out because another machine cannot use them.
var webLANRanges = mustParseCIDRs("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10", "fc00::/7")

// webBaseURL picks the host of the web URL: the first allowed host the root
// permits, then the listen IP when it is specific and inside the root's ranges,
// then the first LAN interface IP, else localhost. A nil listenIP is an
// unspecified listen address.
func webBaseURL(allowed []string, root *x509.Certificate, listenIP net.IP, port string, ifaceIPs []net.IP) string {
	return "https://" + net.JoinHostPort(webBaseHost(allowed, root, listenIP, ifaceIPs), port)
}

func webBaseHost(allowed []string, root *x509.Certificate, listenIP net.IP, ifaceIPs []net.IP) string {
	for _, h := range allowed {
		if h = normalizeHost(h); h != "" && webHostPermitted(root, h) {
			return h
		}
	}
	if webIPPermitted(root, listenIP) {
		return listenIP.String()
	}
	for _, ip := range ifaceIPs {
		for _, n := range webLANRanges {
			if n.Contains(ip) {
				return ip.String()
			}
		}
	}
	return "localhost"
}
