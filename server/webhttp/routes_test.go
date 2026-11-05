package webhttp

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire"
)

type fakeHost struct{ routes []wire.Route }

func (h fakeHost) Settings() *settings.Settings                      { return &settings.Settings{} }
func (h fakeHost) Routes() []wire.Route                              { return h.routes }
func (fakeHost) Snapshot(http.ResponseWriter, *http.Request)         {}
func (fakeHost) RestartCheckRead(http.ResponseWriter, *http.Request) {}
func (fakeHost) RestartWrite(http.ResponseWriter, *http.Request)     {}
func (fakeHost) StreamEvents(http.ResponseWriter, *http.Request)     {}

func tagHandler(tag string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tag)) }
}

// The web mux serves or refuses a route by its web mode.
func TestHandler_ServesRoutesByWebMode(t *testing.T) {
	t.Parallel()
	fake := []wire.Route{
		{Pattern: "GET /v1/fake/none", Handler: tagHandler("socket-none")},
		{Pattern: "GET /v1/fake/read", Handler: tagHandler("socket-read"), Web: wire.WebRead},
		{Pattern: "POST /v1/fake/write", Handler: tagHandler("socket-write"), Web: wire.WebWrite, WebHandler: tagHandler("web-write")},
		{Pattern: "POST /v1/fake/plain", Handler: tagHandler("socket-plain"), Web: wire.WebWrite},
		{Pattern: "GET /fake/public", WebHandler: tagHandler("web-public"), Web: wire.WebPublic},
	}
	w := New(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)), fakeHost{fake})
	web := w.handler()
	tests := []struct {
		name, method, path string
		code               int
		body               string
	}{
		{"none refused", "GET", "/v1/fake/none", 404, ""},
		{"read", "GET", "/v1/fake/read", 200, "socket-read"},
		{"write uses web variant", "POST", "/v1/fake/write", 200, "web-write"},
		{"write falls back to handler", "POST", "/v1/fake/plain", 200, "socket-plain"},
		{"public", "GET", "/fake/public", 200, "web-public"},
	}
	for _, tc := range tests {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
		req.Header.Set("Origin", "http://"+req.Host)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		web.ServeHTTP(rec, req)
		if rec.Code != tc.code || (tc.body != "" && rec.Body.String() != tc.body) {
			t.Errorf("%s: got %d %q, want %d %q", tc.name, rec.Code, rec.Body.String(), tc.code, tc.body)
		}
	}
	if !w.isPublicRoute(httptest.NewRequest("GET", "/fake/public", nil)) {
		t.Error("public route is not served before the login check")
	}
}
