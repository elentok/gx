package web

import (
	"net"
	"net/http"
	"strings"

	"github.com/elentok/gx/server/webtls"
)

const filesPrefix = "/v1/files/"

// IsFileRoute reports whether r is aimed at the hosted-file route, by either
// its escaped or its decoded path, so an encoded prefix cannot dodge the headers.
func IsFileRoute(r *http.Request) bool {
	return strings.HasPrefix(r.URL.EscapedPath(), filesPrefix) || strings.HasPrefix(r.URL.Path, filesPrefix)
}

// FileHeaders sets the security headers every hosted-file response carries. It
// wraps the login gate so the gate's 401 carries them too. It sets no
// Cross-Origin-Resource-Policy and no Access-Control-Allow-Origin.
func FileHeaders(allowed func() []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if IsFileRoute(r) {
			h := w.Header()
			h.Set("Content-Security-Policy", FileCSP(r.Host, allowed()))
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Cache-Control", "private, no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// FileCSP is the policy for a hosted file. connect-src is rebuilt from the
// validated host and a numeric port, never copied raw from the request.
func FileCSP(hostport string, allowed []string) string {
	connect := "'none'"
	if webtls.HostAllowed(hostport, allowed) {
		connect = "https://" + rebuildHost(hostport)
	}
	return "sandbox allow-scripts; frame-ancestors 'self'; connect-src " + connect
}

func rebuildHost(hostport string) string {
	host := webtls.NormalizeHost(hostport)
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if _, port, err := net.SplitHostPort(hostport); err == nil && isNumeric(port) {
		return host + ":" + port
	}
	return host
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
