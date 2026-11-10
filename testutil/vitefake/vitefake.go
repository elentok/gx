// Package vitefake is a scripted stand-in for `pnpm`, for fixtures that run a
// real gx server which launches pnpm. Put the built cmd/pnpm binary first on
// PATH and point ControlEnv at a JSON control file; the program only reads it,
// polling, so a fixture changes behaviour by rewriting the file.
//
// The commands match what server/devvite's pnpm launcher runs:
// `pnpm install --frozen-lockfile` and
// `pnpm exec vite --host H --port N --strictPort`.
package vitefake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

// ControlEnv names the environment variable holding the control file path.
const ControlEnv = "VITEFAKE_CONTROL"

// Control is the control file's content. An absent or empty file means
// install succeeds and vite is ready.
type Control struct {
	// Install is "" or "ok" (exit 0), "hold" (block until changed) or "fail" (exit 1).
	Install string `json:"install,omitempty"`
	// Vite is "" or "ready" (serve), "hold" (never ready) or "crash" (exit 1).
	Vite string `json:"vite,omitempty"`
}

const pollInterval = 20 * time.Millisecond

func readControl(path string) Control {
	var c Control
	if path == "" {
		return c
	}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &c) // a half-written file reads as the previous default
	}
	return c
}

// Run behaves as pnpm with args and returns the exit code.
func Run(ctx context.Context, args []string, controlPath string, out io.Writer) int {
	switch {
	case len(args) >= 1 && args[0] == "install":
		return runInstall(ctx, controlPath, out)
	case len(args) >= 3 && args[0] == "exec" && args[1] == "vite":
		return runVite(ctx, args[2:], controlPath, out)
	}
	fmt.Fprintf(out, "vitefake: unsupported pnpm arguments %q\n", args)
	return 2
}

func runInstall(ctx context.Context, controlPath string, out io.Writer) int {
	fmt.Fprintln(out, "Lockfile is up to date, resolution step is skipped")
	fmt.Fprintln(out, "Progress: resolved 1, reused 1, downloaded 0, added 0")
	for {
		switch readControl(controlPath).Install {
		case "fail":
			fmt.Fprintln(out, "ERR_PNPM_FROZEN_LOCKFILE_WITH_OUTDATED_LOCKFILE  scripted install failure")
			return 1
		case "hold":
			if !sleep(ctx) {
				return 1
			}
		default:
			fmt.Fprintln(out, "Done in 0.1s")
			return 0
		}
	}
}

func runVite(ctx context.Context, args []string, controlPath string, out io.Writer) int {
	host, port, err := viteAddr(args)
	if err != nil {
		fmt.Fprintf(out, "vitefake: %v\n", err)
		return 2
	}
	fmt.Fprintln(out, "VITE v0.0.0-fake")
	var srv *http.Server
	defer func() {
		if srv != nil {
			_ = srv.Close()
		}
	}()
	for {
		switch readControl(controlPath).Vite {
		case "crash":
			fmt.Fprintln(out, "error when starting dev server: scripted crash")
			return 1
		case "hold":
			// Ready is not retractable: hold only delays readiness.
		default:
			if srv == nil {
				ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
				if err != nil {
					fmt.Fprintf(out, "Error: Port %d is already in use\n", port)
					return 1
				}
				srv = &http.Server{Handler: stubHandler()}
				go func() { _ = srv.Serve(ln) }()
				fmt.Fprintf(out, "  ➜  Local:   http://%s:%d/\n", host, port)
			}
		}
		if !sleep(ctx) {
			return 1
		}
	}
}

func stubHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/@vite/client", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		_, _ = io.WriteString(w, "export const vitefake = true\n")
	})
	mux.HandleFunc("/src/main.tsx", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		_, _ = io.WriteString(w, "document.getElementById('root').textContent = 'vitefake'\n")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<!doctype html><html><head><script type="module" src="/@vite/client"></script></head>`+
			`<body><div id="root"></div><script type="module" src="/src/main.tsx"></script></body></html>`)
	})
	return mux
}

// viteAddr reads --host and --port the way the supervisor passes them.
func viteAddr(args []string) (string, int, error) {
	host, port := "127.0.0.1", 0
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--host", "--port":
			if i+1 >= len(args) {
				return "", 0, fmt.Errorf("%s needs a value", args[i])
			}
			if args[i] == "--host" {
				host = args[i+1]
			} else {
				p, err := strconv.Atoi(args[i+1])
				if err != nil {
					return "", 0, fmt.Errorf("bad --port %q", args[i+1])
				}
				port = p
			}
			i++
		}
	}
	if port == 0 {
		return "", 0, errors.New("missing --port")
	}
	return host, port, nil
}

func sleep(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(pollInterval):
		return true
	}
}
