package server

import (
	"net"
	"net/http"
	"strings"
)

// requireHost refuses a request whose Host is not an IP literal, localhost or
// a name in server.web.allowed-hosts. IP literals pass because DNS rebinding
// needs an attacker-controlled name in Host. The list is read per request, so a
// config reload applies without a listener restart. It runs before the token
// check, so a wrong Host answers 403 even without a cookie.
func requireHost(allowed func() []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostAllowed(r.Host, allowed()) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("403: this host is not allowed; add it to server.web.allowed-hosts.\n"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func hostAllowed(hostport string, allowed []string) bool {
	host := normalizeHost(hostport)
	if host == "" {
		return false
	}
	if host == "localhost" || net.ParseIP(host) != nil {
		return true
	}
	for _, name := range allowed {
		if normalizeHost(name) == host {
			return true
		}
	}
	return false
}

// normalizeHost drops the port and IPv6 brackets, lower-cases, and strips one
// trailing dot.
func normalizeHost(hostport string) string {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	return strings.TrimSuffix(strings.ToLower(host), ".")
}
