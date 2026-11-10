package devvite

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"time"
)

// pnpmLauncher is the production Launcher: it runs pnpm with the caller's env.
type pnpmLauncher struct {
	env    []string
	client *http.Client
}

// NewPnpmLauncher returns the Launcher that runs `pnpm install` and
// `pnpm exec vite`, giving each child env as its environment.
func NewPnpmLauncher(env []string) Launcher {
	return &pnpmLauncher{env: env, client: &http.Client{Timeout: 2 * time.Second}}
}

func (p *pnpmLauncher) FreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func (p *pnpmLauncher) command(dir string, out io.Writer, args ...string) *exec.Cmd {
	cmd := exec.Command("pnpm", args...)
	cmd.Dir = dir
	cmd.Env = p.env
	cmd.Stdout, cmd.Stderr = out, out
	configureProcessGroup(cmd)
	return cmd
}

func (p *pnpmLauncher) Install(ctx context.Context, webuiDir string, out io.Writer) error {
	cmd := p.command(webuiDir, out, "install", "--frozen-lockfile")
	if err := cmd.Start(); err != nil {
		return mapStartErr(err)
	}
	proc := &execProcess{cmd: cmd, done: make(chan error, 1)}
	go func() { proc.done <- cmd.Wait() }()
	select {
	case err := <-proc.done:
		return err
	case <-ctx.Done():
		proc.Stop()
		<-proc.done
		return ctx.Err()
	}
}

func (p *pnpmLauncher) Start(_ context.Context, webuiDir string, port int, out io.Writer) (Process, error) {
	cmd := p.command(webuiDir, out, "exec", "vite", "--host", "127.0.0.1", "--port", strconv.Itoa(port), "--strictPort")
	if err := cmd.Start(); err != nil {
		return nil, mapStartErr(err)
	}
	proc := &execProcess{cmd: cmd, done: make(chan error, 1)}
	go func() { proc.done <- cmd.Wait() }()
	return proc, nil
}

func (p *pnpmLauncher) Probe(ctx context.Context, port int) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/", port), nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func mapStartErr(err error) error {
	if errors.Is(err, exec.ErrNotFound) {
		return ErrPnpmNotFound
	}
	return err
}

// execProcess adapts a started command; the done channel holds Wait's result
// once, so Wait may be called by one reader.
type execProcess struct {
	cmd  *exec.Cmd
	done chan error
}

func (e *execProcess) Wait() error { return <-e.done }

// Stop signals the whole process group, then kills it if it lingers.
func (e *execProcess) Stop() {
	signalGroup(e.cmd, false)
	time.AfterFunc(5*time.Second, func() { signalGroup(e.cmd, true) })
}
