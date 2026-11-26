// Package hostfileseed builds the file contents and the second origin of the
// host-files scenario.
package hostfileseed

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net"
	"net/http"
	"strings"
)

const bigLogSize = 3 << 20

// PNG is a small real PNG (a gradient), so a viewer has dimensions to show.
func PNG() ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 40))
	for y := range 40 {
		for x := range 64 {
			img.Set(x, y, color.RGBA{R: uint8(x * 4), G: uint8(y * 6), B: 160, A: 255})
		}
	}
	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	return buf.Bytes(), err
}

// StartOtherOrigin serves one script and one JSON path on another loopback
// port, with no CORS headers, for the rest of the fixture's life. It returns its URL.
func StartOtherOrigin() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/lib.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		_, _ = w.Write([]byte("window.otherScript = true;\n"))
	})
	mux.HandleFunc("/data.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"origin":"other"}`))
	})
	go func() { _ = http.Serve(ln, mux) }() //nolint:gosec // loopback fixture listener
	return "http://" + ln.Addr().String(), nil
}

// Files is every file of demo's repo, by path; the viewer mock is added
// separately because it needs the second origin's URL.
func Files(pic []byte) (map[string][]byte, error) {
	var handler strings.Builder
	handler.WriteString("package files\n\n")
	for i := 3; i <= 60; i++ {
		switch i {
		case 42:
			handler.WriteString("// ServeFile is the line the chat links to (handler.go:42).\n")
		default:
			fmt.Fprintf(&handler, "// line %d\n", i)
		}
	}
	var intro strings.Builder
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&intro, "Filler paragraph %d, so the heading below sits under the fold.\n\n", i)
	}
	plan := "# Host files plan\n\n" +
		"See [the handler](../../server/files/handler.go) and the picture ![pic](pic.png).\n\n" +
		"Jump to [the install steps](#install-steps) or read [other.md#install](other.md#install).\n\n" +
		"```mermaid\nflowchart LR\n  Agent --> Link --> Viewer\n```\n\n" +
		intro.String() + "## Install steps\n\nRun the installer.\n"
	return map[string][]byte{
		"docs/plans/host-files.md": []byte(plan),
		"docs/plans/other.md":      []byte("# Other\n\n## Install\n\nThe install section.\n"),
		"docs/plans/pic.png":       pic,
		".scratch/flow.mmd":        []byte("flowchart LR\n  A[Agent] --> B[gx files url] --> C[Chat link] --> D[Viewer]\n"),
		"server/files/handler.go":  []byte(handler.String()),
		"notes/gone.md":            []byte("# Gone\n\nThe flow deletes this after the chat loads.\n"),
		".env":                     []byte("SECRET=hunter2\n"),
		"big.log":                  bytes.Repeat([]byte("a log line of some length\n"), bigLogSize/26+1)[:bigLogSize],
		"doc.pdf":                  []byte("%PDF-1.1\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 100]>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n"),
		"blob.bin":                 {0, 1, 2, 3, 0, 255, 254, 0, 7},
		"logo.svg":                 []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><script>window.svgRan = true;</script><circle cx="20" cy="20" r="18"/></svg>` + "\n"),
		"mocks/style.css":          []byte("body { font-family: sans-serif; }\n#title { color: rgb(1, 2, 3); }\n"),
		"mocks/data.json":          []byte(`{"same":"origin"}` + "\n"),
		"mocks/shot.png":           pic,
	}, nil
}

// ViewerMock is mocks/04-viewer.html and its app.js. The page records five
// behaviours in #results as JSON: the page loads its CSS, script and image; a
// same-origin fetch of data.json works; a read of /v1/snapshot is refused; a
// fetch to the second origin fails; a script from the second origin loads.
func ViewerMock(other string) (page, app []byte) {
	page = []byte(`<!doctype html>
<html><head><meta charset="utf-8"><title>Viewer mock</title>
<link rel="stylesheet" href="style.css"></head>
<body data-other="` + other + `">
<h1 id="title">Viewer mock</h1>
<img id="shot" src="shot.png" alt="shot">
<pre id="results"></pre>
<script src="app.js"></script>
<script src="` + other + `/lib.js"></script>
</body></html>
`)
	app = []byte(`const results = {};
const other = document.body.dataset.other;
const attempt = (name, p) => p.then((v) => { results[name] = v; }, () => { results[name] = 'failed'; });
Promise.all([
  attempt('sameOriginFetch', fetch('data.json').then((r) => r.json()).then(() => 'ok')),
  attempt('snapshot', fetch('/v1/snapshot').then((r) => (r.ok ? 'read' : 'refused'))),
  attempt('otherOriginFetch', fetch(other + '/data.json').then(() => 'ok')),
]).then(() => {
  window.addEventListener('load', () => {
    results.otherScript = window.otherScript === true ? 'loaded' : 'missing';
    results.image = document.getElementById('shot').naturalWidth > 0 ? 'loaded' : 'missing';
    results.style = getComputedStyle(document.getElementById('title')).color;
    document.getElementById('results').textContent = JSON.stringify(results);
    document.body.dataset.done = '1';
  }, { once: true });
  if (document.readyState === 'complete') window.dispatchEvent(new Event('load'));
});
`)
	return page, app
}
