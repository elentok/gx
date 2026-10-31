package codexdoctor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

// streamLine is one line of a stream fixture: a chunk of raw PTY output.
type streamLine struct {
	T   float64 `json:"t"`
	B64 []byte  `json:"b64"`
}

// ReadStream loads a stream fixture (<dir>/streams/<session>.jsonl).
func ReadStream(path string) ([]Frame, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	var frames []Frame
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var l streamLine
		if err := json.Unmarshal(scanner.Bytes(), &l); err != nil {
			return nil, err
		}
		frames = append(frames, Frame{T: l.T, Data: l.B64})
	}
	return frames, scanner.Err()
}

// Record wraps src so every live session it produces is also written to dir in
// fixture format: frames to streams/<session>.jsonl and rollouts to
// rollouts/<base name>. The rollout's own session_meta carries the version.
// The rollouts session is skipped: it only repeats the shipped fixtures.
func Record(src Source, dir string) Source {
	inner := src.Session
	src.Session = func(name string) (Recording, error) {
		rec, err := inner(name)
		if err != nil || name == RolloutsSession {
			return rec, err
		}
		if err := writeFixture(dir, name, rec); err != nil {
			return Recording{}, err
		}
		return rec, nil
	}
	return src
}

func writeFixture(dir, name string, rec Recording) error {
	if len(rec.Frames) > 0 {
		var buf bytes.Buffer
		for _, f := range rec.Frames {
			line, err := json.Marshal(streamLine{T: f.T, B64: f.Data})
			if err != nil {
				return err
			}
			buf.Write(line)
			buf.WriteByte('\n')
		}
		if err := writeFile(filepath.Join(dir, "streams", name+".jsonl"), buf.Bytes()); err != nil {
			return err
		}
	}
	for _, r := range rec.Rollouts {
		if err := writeFile(filepath.Join(dir, "rollouts", filepath.Base(r.Name)), r.Data); err != nil {
			return err
		}
	}
	return nil
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
