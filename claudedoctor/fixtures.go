package claudedoctor

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
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
// version that produced it, then that session's raw events.
type fixtureHeader struct {
	ClaudeVersion string `json:"claude_version"`
}

// FixtureSource is the recorded fixtures shipped in testdata/.
func FixtureSource() (Source, error) {
	sub, err := fs.Sub(embedded, "testdata")
	if err != nil {
		return Source{}, err
	}
	return LoadFixtures(sub)
}

// LoadFixtures reads every <session>.jsonl in fsys. The source's
// ClaudeVersion is the oldest header, so a stale fixture is never hidden by
// a fresher one.
func LoadFixtures(fsys fs.FS) (Source, error) {
	paths, err := fs.Glob(fsys, "*.jsonl")
	if err != nil {
		return Source{}, err
	}
	sessions := map[string][]json.RawMessage{}
	var oldest string
	for _, p := range paths {
		version, events, err := readFixture(fsys, p)
		if err != nil {
			return Source{}, fmt.Errorf("fixture %s: %w", p, err)
		}
		if oldest == "" || compareVersions(version, oldest) < 0 {
			oldest = version
		}
		sessions[strings.TrimSuffix(p, ".jsonl")] = events
	}
	return Source{
		ClaudeVersion: oldest,
		Session: func(name string) ([]json.RawMessage, error) {
			events, ok := sessions[name]
			if !ok {
				return nil, fmt.Errorf("no fixture for session %s", name)
			}
			return events, nil
		},
	}, nil
}

func readFixture(fsys fs.FS, path string) (string, []json.RawMessage, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return "", nil, err
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	var header fixtureHeader
	if !scanner.Scan() || json.Unmarshal(scanner.Bytes(), &header) != nil || header.ClaudeVersion == "" {
		return "", nil, fmt.Errorf("first line must be a {\"claude_version\":...} header")
	}
	var events []json.RawMessage
	for scanner.Scan() {
		if line := bytes.TrimSpace(scanner.Bytes()); len(line) > 0 {
			events = append(events, json.RawMessage(bytes.Clone(line)))
		}
	}
	return header.ClaudeVersion, events, scanner.Err()
}

// Record wraps src so every session it produces is also written to
// dir/<session>.jsonl in fixture format, ready to copy into testdata/.
func Record(src Source, dir string) Source {
	inner := src.Session
	src.Session = func(name string) ([]json.RawMessage, error) {
		events, err := inner(name)
		if err != nil {
			return nil, err
		}
		if err := writeFixture(filepath.Join(dir, name+".jsonl"), src.ClaudeVersion, events); err != nil {
			return nil, err
		}
		return events, nil
	}
	return src
}

func writeFixture(path, version string, events []json.RawMessage) error {
	header, err := json.Marshal(fixtureHeader{ClaudeVersion: version})
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.Write(header)
	buf.WriteByte('\n')
	for _, e := range events {
		buf.Write(e)
		buf.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
