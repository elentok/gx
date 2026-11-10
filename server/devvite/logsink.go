package devvite

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
)

const (
	logFileName = "dev-vite.log"
	// logMaxBytes bounds the live file; the previous generation is kept as
	// logFileName+".1", so the folder holds at most about twice this.
	logMaxBytes = 5 << 20
)

// logSink is the io.Writer that install and Vite output goes to: a rotating
// file plus a bounded buffer of the last complete lines.
type logSink struct {
	mu      sync.Mutex
	path    string
	f       *os.File
	size    int64
	max     int
	lines   []string
	partial []byte
}

func openLogSink(dir string, maxLines int) (*logSink, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	l := &logSink{path: filepath.Join(dir, logFileName), max: maxLines}
	if err := l.open(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *logSink) open() error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	l.f, l.size = f, info.Size()
	return nil
}

func (l *logSink) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.size > 0 && l.size+int64(len(p)) > logMaxBytes {
		l.f.Close()
		if err := os.Rename(l.path, l.path+".1"); err != nil {
			return 0, err
		}
		if err := l.open(); err != nil {
			return 0, err
		}
	}
	n, err := l.f.Write(p)
	l.size += int64(n)
	l.addLines(p)
	return n, err
}

func (l *logSink) addLines(p []byte) {
	data := append(l.partial, p...)
	l.partial = nil
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			break
		}
		l.lines = append(l.lines, string(bytes.TrimSuffix(data[:i], []byte("\r"))))
		data = data[i+1:]
	}
	l.partial = append([]byte(nil), data...)
	if extra := len(l.lines) - l.max; extra > 0 {
		l.lines = append([]string(nil), l.lines[extra:]...)
	}
}

func (l *logSink) tail() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}

func (l *logSink) close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.Close()
}
