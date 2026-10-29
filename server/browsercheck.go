package server

import (
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

const browserRefusalLogEvery = time.Minute

// refuseBrowsers wraps the loopback TCP listener's handler. A page in a
// browser on this machine can reach 127.0.0.1, and scripts send none of
// Origin, Sec-Fetch-Site or a foreign Host, so those get 403.
func refuseBrowsers(next http.Handler, log *slog.Logger) http.Handler {
	var lastLog atomic.Int64
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reason := browserReason(r); reason != "" {
			now := time.Now().UnixNano()
			last := lastLog.Load()
			if now-last >= int64(browserRefusalLogEvery) && lastLog.CompareAndSwap(last, now) {
				log.Warn("refused browser request on the loopback TCP listener", "reason", reason, "host", r.Host)
			}
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func browserReason(r *http.Request) string {
	if r.Header.Get("Origin") != "" {
		return "origin header"
	}
	if r.Header.Get("Sec-Fetch-Site") != "" {
		return "sec-fetch-site header"
	}
	host := normalizeHost(r.Host)
	if host != "localhost" && net.ParseIP(host) == nil {
		return "non-loopback host"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() {
		return "non-loopback host"
	}
	return ""
}
