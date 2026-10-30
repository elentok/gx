package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/elentok/gx/nativerunner"
)

// shutdown stops the server before its detached holders, so it cannot launch
// replacements during cleanup. The holder protocol stops the child's entire
// process group and lets the holder flush its output before exiting.
func (f *fixture) shutdown(srv *exec.Cmd, exited <-chan error) error {
	_ = srv.Process.Signal(syscall.SIGTERM)
	select {
	case <-exited:
	case <-time.After(15 * time.Second):
		_ = srv.Process.Kill()
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			return errors.New("fixture server did not exit after SIGKILL")
		}
	}
	return f.stopHolders()
}

func (f *fixture) stopHolders() error {
	var sockets []string
	var errs []error
	err := filepath.WalkDir(f.root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if !os.IsNotExist(err) {
				errs = append(errs, err)
			}
			return nil
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.Name() != nativerunner.HolderSocketFile || entry.Type()&os.ModeSocket == 0 {
			return nil
		}
		holder, err := nativerunner.DialHolder(filepath.Dir(path))
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED) {
			return nil // A holder that already exited can leave a stale socket.
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("fixture holder %s: %w", path, err))
			return nil
		}
		if err := holder.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("stop fixture holder %s: %w", path, err))
			return nil
		}
		sockets = append(sockets, path)
		return nil
	})
	if err != nil {
		errs = append(errs, err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for _, socket := range sockets {
		for {
			_, err := os.Stat(socket)
			if os.IsNotExist(err) {
				break
			}
			if err != nil || time.Now().After(deadline) {
				errs = append(errs, fmt.Errorf("fixture holder did not stop: %s", socket))
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	return errors.Join(errs...)
}
