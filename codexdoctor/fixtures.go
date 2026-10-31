package codexdoctor

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/nativerunner"
)

// The fixtures are embedded so the doctor can warn when installed Codex is
// newer than the Codex that recorded them.
//
//go:embed testdata/rollouts/*.jsonl testdata/streams/*.jsonl
var embedded embed.FS

// FixtureSource is the recorded fixtures shipped in testdata/.
func FixtureSource() (Source, error) {
	sub, err := fs.Sub(embedded, "testdata/rollouts")
	if err != nil {
		return Source{}, err
	}
	return LoadFixtures(sub)
}

// LoadFixtures reads every rollout in fsys. The source's CodexVersion is the
// oldest session_meta cli_version, so a stale fixture is never hidden by a
// fresher one. A file without a session_meta line (a lone token_count sample)
// carries no version.
func LoadFixtures(fsys fs.FS) (Source, error) {
	paths, err := fs.Glob(fsys, "*.jsonl")
	if err != nil {
		return Source{}, err
	}
	var rec Recording
	var oldest string
	for _, p := range paths {
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return Source{}, fmt.Errorf("fixture %s: %w", p, err)
		}
		rec.Rollouts = append(rec.Rollouts, Rollout{Name: p, Data: data})
		if v := rolloutVersion(data); v != "" && (oldest == "" || nativerunner.CompareVersions(v, oldest) < 0) {
			oldest = v
		}
	}
	return Source{
		CodexVersion: oldest,
		Session: func(name string) (Recording, error) {
			if name != RolloutsSession {
				return Recording{}, fmt.Errorf("no fixture for session %s", name)
			}
			return rec, nil
		},
	}, nil
}

// rolloutVersion is the first session_meta line's payload.cli_version.
func rolloutVersion(data []byte) string {
	for _, raw := range bytes.Split(data, []byte("\n")) {
		var l struct {
			Type    string `json:"type"`
			Payload struct {
				CLIVersion string `json:"cli_version"`
			} `json:"payload"`
		}
		if json.Unmarshal(raw, &l) == nil && l.Type == "session_meta" {
			return l.Payload.CLIVersion
		}
	}
	return ""
}

// WithLiveRollout adds the newest rollout under codexHome/sessions to the
// fixture rollouts. It returns ErrNoSession when there is none yet.
func WithLiveRollout(codexHome string, fixtures Recording) (Recording, error) {
	newest, err := newestRollout(filepath.Join(codexHome, "sessions"))
	if err != nil {
		return Recording{}, err
	}
	data, err := os.ReadFile(newest)
	if err != nil {
		return Recording{}, err
	}
	rec := Recording{Rollouts: append([]Rollout{{Name: newest, Data: data}}, fixtures.Rollouts...)}
	return rec, nil
}

// newestRollout is the most recently modified rollout-*.jsonl under root.
func newestRollout(root string) (string, error) {
	var newest string
	var newestTime int64
	err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() || !strings.HasPrefix(e.Name(), "rollout-") || !strings.HasSuffix(e.Name(), ".jsonl") {
			return nil
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		if t := info.ModTime().UnixNano(); newest == "" || t > newestTime {
			newest, newestTime = path, t
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) || (err == nil && newest == "") {
		return "", ErrNoSession
	}
	return newest, err
}
