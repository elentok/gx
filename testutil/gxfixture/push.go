package main

import (
	"path/filepath"
	"slices"

	"github.com/elentok/gx/testutil/gxfixture/pushcapture"
)

// startPushService starts the fake push service for a PushService scenario and
// points the server at it. The returned func stops it. A function, not a method,
// to keep fixture under the sizeguard method limit.
func startPushService(f *fixture, o *opts, sc scenario) (func(), error) {
	if !sc.PushService {
		return func() {}, nil
	}
	f.pushCapture = filepath.Join(f.root, "push-capture.jsonl")
	f.pushControl = filepath.Join(f.root, "push-control")
	srv, err := pushcapture.Start(f.pushCapture, f.pushControl)
	if err != nil {
		return nil, err
	}
	f.pushURL = srv.URL()
	o.configs = append(slices.Clone(o.configs), "server.web.push-contact-email=fixture@example.com")
	f.env = append(f.env, "GX_PUSH_ALLOW_HTTP_ENDPOINTS=1")
	return func() { _ = srv.Close() }, nil
}
