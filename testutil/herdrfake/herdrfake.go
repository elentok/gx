// Package herdrfake provides a process-boundary fake `herdr` executable that
// tests can put first in PATH, so production code that shells out to the real
// `herdr` CLI (via exec.Command("herdr", ...)) reaches the fake instead.
//
// The fake executable is the test binary itself, re-invoked under a dedicated
// environment variable. Each invocation dials a Unix socket back to the
// parent test process, sends its argv, and prints whatever the test's
// Handler decides to answer. It never execs or dials anything else, so it
// cannot reach a real herdr installation or user session.
package herdrfake

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// helperSocketEnv names the environment variable a helper invocation reads to
// find the coordinator's Unix socket. Its presence is what distinguishes a
// helper re-invocation of the test binary from a normal `go test` run.
const helperSocketEnv = "GX_HERDR_FAKE_HELPER_SOCKET"

// Handler answers one herdr invocation. output is written verbatim to the
// helper's stdout (mirroring exec.Cmd.CombinedOutput, which callers in this
// codebase rely on) and exitCode becomes the helper process's exit status.
// Use Result for a success envelope or CommandError for a failure.
//
// Handler runs on a goroutine other than the test's own, so it must report
// failures with t.Error/t.Errorf, never t.Fatal/t.Fatalf.
type Handler func(argv []string) (output []byte, exitCode int)

// Result builds a herdr success envelope — {"result": v} — matching the real
// CLI's response shape, with exit code 0.
func Result(v any) (output []byte, exitCode int) {
	b, err := json.Marshal(struct {
		Result any `json:"result"`
	}{Result: v})
	if err != nil {
		panic(fmt.Sprintf("herdrfake.Result: %v", err))
	}
	return b, 0
}

// CommandError builds a herdr command failure: msg as the combined output and
// exit code 1, matching how the real CLI reports a failed command.
func CommandError(msg string) (output []byte, exitCode int) {
	return []byte(msg), 1
}

type request struct {
	Argv []string `json:"argv"`
}

type response struct {
	Output   []byte `json:"output"`
	ExitCode int    `json:"exit_code"`
}

// Coordinator is the parent-side half of the process boundary, returned by
// Start. It exists so callers (e.g. ticket 04's stateful coordinator) have a
// handle to build on; this package's own Start already wires up its
// lifecycle, so most tests never touch it directly.
type Coordinator struct {
	ln net.Listener
}

// Start puts a fake `herdr` executable first in PATH for the duration of the
// test, backed by handler. It calls t.Setenv to publish the socket path and
// mutate PATH, which panics if the test has already called t.Parallel —
// process-wide environment changes and parallel subtests don't mix, so tests
// using Start must stay non-parallel.
func Start(t *testing.T, handler Handler) *Coordinator {
	t.Helper()

	sockDir, err := os.MkdirTemp("", "hf")
	if err != nil {
		t.Fatalf("herdrfake: create socket dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "s")

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("herdrfake: listen on %s: %v", sockPath, err)
	}
	t.Cleanup(func() { ln.Close() })

	c := &Coordinator{ln: ln}
	go c.serve(handler)

	binDir := t.TempDir()
	fakeExe := filepath.Join(binDir, "herdr")
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("herdrfake: resolve test binary: %v", err)
	}
	if err := os.Symlink(self, fakeExe); err != nil {
		t.Fatalf("herdrfake: symlink fake herdr executable: %v", err)
	}

	t.Setenv(helperSocketEnv, sockPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	return c
}

// serve accepts one connection per helper invocation until the listener
// closes (on test cleanup).
func (c *Coordinator) serve(handler Handler) {
	for {
		conn, err := c.ln.Accept()
		if err != nil {
			return
		}
		go handleConn(conn, handler)
	}
}

func handleConn(conn net.Conn, handler Handler) {
	defer conn.Close()

	var req request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return
	}

	output, exitCode := handler(req.Argv)

	_ = json.NewEncoder(conn).Encode(&response{Output: output, ExitCode: exitCode})
}

// RunHelperProcess must be the first thing called in a package's TestMain,
// before m.Run. If the process was launched as the fake herdr helper (i.e.
// GX_HERDR_FAKE_HELPER_SOCKET is set), it forwards its complete argv to the
// coordinator over that Unix socket, writes back whatever output the
// coordinator answers with, exits with the coordinator's exit code, and never
// returns. Otherwise it's a no-op and the normal test run proceeds.
//
//	func TestMain(m *testing.M) {
//		herdrfake.RunHelperProcess()
//		os.Exit(m.Run())
//	}
func RunHelperProcess() {
	sockPath := os.Getenv(helperSocketEnv)
	if sockPath == "" {
		return
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "herdrfake: dial %s: %v\n", sockPath, err)
		os.Exit(1)
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(&request{Argv: os.Args[1:]}); err != nil {
		fmt.Fprintf(os.Stderr, "herdrfake: send request: %v\n", err)
		os.Exit(1)
	}

	var resp response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		fmt.Fprintf(os.Stderr, "herdrfake: read response: %v\n", err)
		os.Exit(1)
	}

	os.Stdout.Write(resp.Output)
	os.Exit(resp.ExitCode)
}
