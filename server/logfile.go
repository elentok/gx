package server

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	logFileName = "server.log"
	// logMaxBytes and logMaxFiles bound server logs to ~30 MB: the live file
	// plus two rotated generations.
	logMaxBytes = 10 << 20
	logMaxFiles = 3
)

// LogPath is the server's slog JSON file inside stateDir.
func LogPath(stateDir string) string { return filepath.Join(stateDir, logFileName) }

// rotatingFile is an append-only file that rolls to path.1, path.2, ... once a
// write would push it past maxBytes. Only maxFiles files ever exist.
type rotatingFile struct {
	path     string
	maxBytes int64
	maxFiles int

	mu   sync.Mutex
	f    *os.File
	size int64
}

func openRotatingFile(path string, maxBytes int64, maxFiles int) (*rotatingFile, error) {
	r := &rotatingFile{path: path, maxBytes: maxBytes, maxFiles: maxFiles}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *rotatingFile) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	r.f, r.size = f, info.Size()
	return nil
}

func (r *rotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Rotating an empty file would loop forever on an oversized line.
	if r.size > 0 && r.size+int64(len(p)) > r.maxBytes {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *rotatingFile) rotate() error {
	if err := r.f.Close(); err != nil {
		return err
	}
	// Oldest generation is overwritten by the rename chain.
	for i := r.maxFiles - 1; i >= 1; i-- {
		from := r.path
		if i > 1 {
			from = fmt.Sprintf("%s.%d", r.path, i-1)
		}
		if err := os.Rename(from, fmt.Sprintf("%s.%d", r.path, i)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return r.open()
}

func (r *rotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.f.Close()
}
