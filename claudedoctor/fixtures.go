package claudedoctor

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The fixtures are embedded so the doctor can warn when installed claude is
// newer than the claude that recorded them.
//
//go:embed testdata/*.jsonl
var embedded embed.FS

// A fixture file is one canned session: a header line naming the claude
// version that produced it, then that session's raw events. Its PTY screen
// frames (one JSON string per line) and status-file snapshots live in
// optional sidecar files, so a headless session keeps its plain format.
type fixtureHeader struct {
	ClaudeVersion string `json:"claude_version"`
}

const (
	framesSuffix    = ".frames.jsonl"
	snapshotsSuffix = ".status.jsonl"
)

// FixtureSource is the recorded fixtures shipped in testdata/.
func FixtureSource() (Source, error) {
	sub, err := fs.Sub(embedded, "testdata")
	if err != nil {
		return Source{}, err
	}
	return LoadFixtures(sub)
}

// LoadFixtures reads every <session>.jsonl in fsys, plus its sidecars. The
// source's ClaudeVersion is the oldest header, so a stale fixture is never
// hidden by a fresher one.
func LoadFixtures(fsys fs.FS) (Source, error) {
	paths, err := fs.Glob(fsys, "*.jsonl")
	if err != nil {
		return Source{}, err
	}
	sessions := map[string]Recording{}
	var oldest string
	for _, p := range paths {
		if strings.HasSuffix(p, framesSuffix) || strings.HasSuffix(p, snapshotsSuffix) {
			continue
		}
		name := strings.TrimSuffix(p, ".jsonl")
		version, rec, err := readFixture(fsys, name)
		if err != nil {
			return Source{}, fmt.Errorf("fixture %s: %w", p, err)
		}
		if oldest == "" || compareVersions(version, oldest) < 0 {
			oldest = version
		}
		sessions[name] = rec
	}
	return Source{
		ClaudeVersion: oldest,
		Session: func(name string) (Recording, error) {
			rec, ok := sessions[name]
			if !ok {
				return Recording{}, fmt.Errorf("no fixture for session %s", name)
			}
			return rec, nil
		},
	}, nil
}

func readFixture(fsys fs.FS, name string) (string, Recording, error) {
	lines, err := readLines(fsys, name+".jsonl")
	if err != nil {
		return "", Recording{}, err
	}
	var header fixtureHeader
	if len(lines) == 0 || json.Unmarshal(lines[0], &header) != nil || header.ClaudeVersion == "" {
		return "", Recording{}, fmt.Errorf("first line must be a {\"claude_version\":...} header")
	}
	rec := Recording{Events: lines[1:]}

	frames, err := readLines(fsys, name+framesSuffix)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", Recording{}, err
	}
	for _, f := range frames {
		var frame string
		if err := json.Unmarshal(f, &frame); err != nil {
			return "", Recording{}, fmt.Errorf("%s%s: %w", name, framesSuffix, err)
		}
		rec.Frames = append(rec.Frames, frame)
	}

	rec.Snapshots, err = readLines(fsys, name+snapshotsSuffix)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", Recording{}, err
	}
	return header.ClaudeVersion, rec, nil
}

// readLines returns path's non-blank lines.
func readLines(fsys fs.FS, path string) ([]json.RawMessage, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	var lines []json.RawMessage
	for scanner.Scan() {
		if line := bytes.TrimSpace(scanner.Bytes()); len(line) > 0 {
			lines = append(lines, json.RawMessage(bytes.Clone(line)))
		}
	}
	return lines, scanner.Err()
}

// Record wraps src so every session it produces is also written to dir in
// fixture format (<session>.jsonl plus any sidecars), ready to copy into
// testdata/.
func Record(src Source, dir string) Source {
	inner := src.Session
	src.Session = func(name string) (Recording, error) {
		rec, err := inner(name)
		if err != nil {
			return Recording{}, err
		}
		if err := writeFixture(dir, name, src.ClaudeVersion, rec); err != nil {
			return Recording{}, err
		}
		return rec, nil
	}
	return src
}

func writeFixture(dir, name, version string, rec Recording) error {
	header, err := json.Marshal(fixtureHeader{ClaudeVersion: version})
	if err != nil {
		return err
	}
	frames := make([]json.RawMessage, len(rec.Frames))
	for i, f := range rec.Frames {
		if frames[i], err = json.Marshal(f); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeLines(filepath.Join(dir, name+".jsonl"), append([]json.RawMessage{header}, rec.Events...)); err != nil {
		return err
	}
	if err := writeLines(filepath.Join(dir, name+framesSuffix), frames); err != nil {
		return err
	}
	return writeLines(filepath.Join(dir, name+snapshotsSuffix), rec.Snapshots)
}

// writeLines writes one line per entry; it writes no file for no entries.
func writeLines(path string, lines []json.RawMessage) error {
	if len(lines) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for _, l := range lines {
		buf.Write(l)
		buf.WriteByte('\n')
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
