package cmdbase

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/projectlock"
)

// Env is the runtime context every command package shares. Tests inject fakes
// into its fields; the zero value of each optional field means the real one.
type Env struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Getwd  func() (string, error)
	// Env is the process environment every config dir is resolved from.
	Env        []string
	Getenv     func(string) string
	LoadConfig func() (config.Config, error)
	// NewClient dials the server; nil means the real one (tests inject a fake).
	NewClient func() (*apiclient.Client, error)
}

// Dirs resolves the config, data and state directories from the env.
func (e Env) Dirs() config.Dirs { return config.DirsFrom(e.Env) }

// Dial returns the injected server client factory, or the real one.
func (e Env) Dial() func() (*apiclient.Client, error) {
	if e.NewClient != nil {
		return e.NewClient
	}
	return func() (*apiclient.Client, error) { return ServerClient(e.Dirs()) }
}

// AbsPath makes arg absolute: "~" and "~/x" against HOME, a relative path
// against the caller's folder. The server only ever sees absolute paths.
func (e Env) AbsPath(arg string) (string, error) {
	if arg == "~" || strings.HasPrefix(arg, "~/") {
		home, err := e.Dirs().Home()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, arg[1:]), nil
	}
	if filepath.IsAbs(arg) {
		return filepath.Clean(arg), nil
	}
	cwd, err := e.Cwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, arg), nil
}

// Cwd is the caller's folder: the injected Getwd, or the real one.
func (e Env) Cwd() (string, error) {
	if e.Getwd != nil {
		return e.Getwd()
	}
	return os.Getwd()
}

// IsTerminal reports whether stdin is a terminal, by the given hook; a nil
// hook means no.
func IsTerminal(stdinIsTerminal func() bool) bool {
	return stdinIsTerminal != nil && stdinIsTerminal()
}

// ReadBodyArg returns arg, or all of stdin when arg is "-".
func ReadBodyArg(stdin io.Reader, arg string) (string, error) {
	if arg != "-" {
		return arg, nil
	}
	if stdin == nil {
		stdin = os.Stdin
	}
	b, err := io.ReadAll(stdin)
	return string(b), err
}

// RegisterMigrationWaitNotice prints a stderr line when a command waits on a
// project migration. Stderr, because agents parse stdout. The server logs
// instead, so runServer unregisters it.
func RegisterMigrationWaitNotice(w io.Writer) (restore func()) {
	return projectlock.SetWaitHook(func(project string) {
		fmt.Fprintf(w, "waiting for the migration of %s\n", project)
	})
}
