package server

import (
	"net"
	"testing"
	"time"
)

func TestWebBaseURL(t *testing.T) {
	root, err := newWebRoot([]string{"gx.example.com"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	lan := []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("192.168.1.7")}
	tests := []struct {
		name    string
		allowed []string
		listen  string
		ifaces  []net.IP
		want    string
	}{
		{"allowed host inside constraints", []string{"gx.example.com"}, "10.0.0.5", lan, "https://gx.example.com:8443"},
		{"allowed host outside constraints is skipped", []string{"evil.test"}, "10.0.0.5", lan, "https://10.0.0.5:8443"},
		{"first permitted allowed host wins", []string{"evil.test", "gx.example.com"}, "", lan, "https://gx.example.com:8443"},
		{"private listen IP", nil, "192.168.4.2", lan, "https://192.168.4.2:8443"},
		{"public listen IP falls to interfaces", nil, "8.8.4.4", lan, "https://192.168.1.7:8443"},
		{"IPv6 listen IP is bracketed", nil, "fd00::1", lan, "https://[fd00::1]:8443"},
		{"unspecified listen with private interface", nil, "", lan, "https://192.168.1.7:8443"},
		{"unspecified listen without private interface", nil, "", []net.IP{net.ParseIP("8.8.8.8")}, "https://localhost:8443"},
		{"IPv6 interface is bracketed", nil, "", []net.IP{net.ParseIP("fd12::9")}, "https://[fd12::9]:8443"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := webBaseURL(tc.allowed, root.Cert, net.ParseIP(tc.listen), "8443", tc.ifaces)
			if got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
