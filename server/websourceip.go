package server

import (
	"log/slog"
	"net"
	"net/netip"
	"sync"
	"time"
)

// webRefuseLogEvery is the shortest gap between two log lines for one refused IP.
const webRefuseLogEvery = time.Minute

// webRefuseLogMax bounds the gate's memory against a scan from many addresses.
const webRefuseLogMax = 1024

var webAllowedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fc00::/7"),
}

// webSourceAllowed reports whether a peer address may reach the web listener.
// An IPv4-mapped IPv6 address is judged as the IPv4 address inside it.
func webSourceAllowed(ip netip.Addr) bool {
	ip = ip.Unmap()
	for _, p := range webAllowedPrefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// logGate lets one key through at most once per interval.
type logGate struct {
	mu       sync.Mutex
	now      func() time.Time
	interval time.Duration
	last     map[string]time.Time
}

func newLogGate(interval time.Duration, now func() time.Time) *logGate {
	return &logGate{now: now, interval: interval, last: map[string]time.Time{}}
}

func (g *logGate) allow(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	t := g.now()
	if prev, ok := g.last[key]; ok && t.Sub(prev) < g.interval {
		return false
	}
	if len(g.last) >= webRefuseLogMax {
		for k, v := range g.last {
			if t.Sub(v) >= g.interval {
				delete(g.last, k)
			}
		}
	}
	g.last[key] = t
	return true
}

// sourceIPListener closes connections from outside the allowed ranges as soon
// as they are accepted, before the first-byte split and before TLS. It judges
// the TCP peer address only, never a header.
type sourceIPListener struct {
	net.Listener
	log  *slog.Logger
	gate *logGate
}

func newSourceIPListener(ln net.Listener, log *slog.Logger, now func() time.Time) *sourceIPListener {
	return &sourceIPListener{Listener: ln, log: log, gate: newLogGate(webRefuseLogEvery, now)}
}

func (l *sourceIPListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		ip, ok := peerIP(c.RemoteAddr())
		if ok && webSourceAllowed(ip) {
			return c, nil
		}
		c.Close()
		key := ip.String()
		if !ok {
			key = "unknown"
		}
		if l.gate.allow(key) {
			l.log.Warn("web connection refused: source IP not allowed", "ip", key)
		}
	}
}

func peerIP(a net.Addr) (netip.Addr, bool) {
	if t, ok := a.(*net.TCPAddr); ok {
		ip, ok := netip.AddrFromSlice(t.IP)
		return ip.Unmap(), ok
	}
	if a == nil {
		return netip.Addr{}, false
	}
	ap, err := netip.ParseAddrPort(a.String())
	return ap.Addr(), err == nil
}
