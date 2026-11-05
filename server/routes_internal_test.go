package server

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"
)

func tagHandler(tag string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tag)) }
}

// A route a subsystem lists is registered on the socket mux, shows up in
// RoutePatterns, and is served or refused on the web mux by its web mode.
func TestSubsystemRoutes_Assembly(t *testing.T) {
	fake := []wire.Route{
		{Pattern: "GET /v1/fake/none", Handler: tagHandler("socket-none")},
		{Pattern: "GET /v1/fake/read", Handler: tagHandler("socket-read"), Web: wire.WebRead},
		{Pattern: "POST /v1/fake/write", Handler: tagHandler("socket-write"), Web: wire.WebWrite, WebHandler: tagHandler("web-write")},
		{Pattern: "POST /v1/fake/plain", Handler: tagHandler("socket-plain"), Web: wire.WebWrite},
		{Pattern: "GET /fake/public", WebHandler: tagHandler("web-public"), Web: wire.WebPublic},
	}
	old := subsystemRoutes
	subsystemRoutes = func(*Server) []wire.Route { return fake }
	t.Cleanup(func() { subsystemRoutes = old })

	patterns := RoutePatterns()
	for _, want := range []string{"GET /v1/fake/none", "GET /v1/fake/read", "POST /v1/fake/write", "POST /v1/fake/plain"} {
		if !slices.Contains(patterns, want) {
			t.Errorf("RoutePatterns lacks %s", want)
		}
	}
	if slices.Contains(patterns, "GET /fake/public") {
		t.Error("a web-only route is a socket pattern")
	}
	if !slices.Contains(WebWritePatterns(), "POST /v1/fake/write") || slices.Contains(WebWritePatterns(), "GET /fake/public") {
		t.Errorf("WebWritePatterns = %v", WebWritePatterns())
	}
	if !slices.Contains(WebPublicGetPatterns(), "GET /fake/public") {
		t.Errorf("WebPublicGetPatterns = %v", WebPublicGetPatterns())
	}
	if !slices.ContainsFunc(WebOnlyRoutes(), func(r WebOnlyRoute) bool { return r.Pattern == "GET /fake/public" && r.Public }) {
		t.Errorf("WebOnlyRoutes lacks the public route: %v", WebOnlyRoutes())
	}

	s := &Server{routes: fake}
	socket := http.NewServeMux()
	registerRoutes(socket, s.routes)
	call := func(h http.Handler, method, path string) (int, string) {
		req := httptest.NewRequest(method, path, strings.NewReader("{}"))
		req.Header.Set("Origin", "http://"+req.Host)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	tests := []struct {
		name, method, path string
		handler            http.Handler
		code               int
		body               string
	}{
		{"socket none", "GET", "/v1/fake/none", socket, 200, "socket-none"},
		{"socket write", "POST", "/v1/fake/write", socket, 200, "socket-write"},
		{"public not on socket", "GET", "/fake/public", socket, 404, ""},
	}
	for _, tc := range tests {
		code, body := call(tc.handler, tc.method, tc.path)
		if code != tc.code || (tc.body != "" && body != tc.body) {
			t.Errorf("%s: got %d %q, want %d %q", tc.name, code, body, tc.code, tc.body)
		}
	}
}
