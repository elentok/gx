package chat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/transcript"
)

// subagentRef is where one subagent call's file is and what its sidecar says.
type subagentRef struct {
	Path          string
	AgentID       string
	ParentAgentID string
	AgentType     string
	// Finished is true once the sidecar has a "finished" key. The value type
	// is unverified, so only the key counts.
	Finished bool
}

// subagentMeta is the sidecar agent-<id>.meta.json next to a subagent file.
type subagentMeta struct {
	ToolUseID     string          `json:"toolUseId"`
	AgentType     string          `json:"agentType"`
	ParentAgentID string          `json:"parentAgentId"`
	Finished      json.RawMessage `json:"finished"`
}

func readSubagentMeta(path string) (subagentMeta, error) {
	var m subagentMeta
	b, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return subagentMeta{}, err
	}
	return m, nil
}

// subagentScan remembers, per subagents/ directory, which agent id each call id
// belongs to. A new file changes the directory's mtime, which ends the entry.
type subagentScan struct {
	mu   sync.Mutex
	dirs map[string]scannedDir
}

type scannedDir struct {
	mtime  time.Time
	byCall map[string]string
}

func (s *subagentScan) agentFor(dir, callID string, scan func(dir string) map[string]string) string {
	info, err := os.Stat(dir)
	if err != nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.dirs[dir]
	if !ok || !e.mtime.Equal(info.ModTime()) {
		e = scannedDir{mtime: info.ModTime(), byCall: scan(dir)}
		if s.dirs == nil {
			s.dirs = map[string]scannedDir{}
		}
		s.dirs[dir] = e
	}
	return e.byCall[callID]
}

func (c *Chat) scanSubagentDir(dir string) map[string]string {
	if c.reads.subagentScan != nil {
		c.reads.subagentScan(dir)
	}
	out := map[string]string{}
	names, _ := filepath.Glob(filepath.Join(dir, "agent-*.meta.json"))
	for _, name := range names {
		m, err := readSubagentMeta(name)
		if err != nil || m.ToolUseID == "" {
			continue
		}
		base := filepath.Base(name)
		out[m.ToolUseID] = strings.TrimSuffix(strings.TrimPrefix(base, "agent-"), ".meta.json")
	}
	return out
}

func validAgentID(id string) bool {
	return id != "" && id != "." && id != ".." && filepath.Base(id) == id
}

// claudeSubagent is the one lookup from a Claude Agent call to its file. A
// record with an agent id takes the launch's output path, else the file next
// to the parent transcript. A foreground call that still runs has no id yet,
// so its sidecar is found by toolUseId, at any depth, since nested agents sit
// flat in the same directory. The strings come from transcript text, so
// unsafe shapes give false.
func (c *Chat) claudeSubagent(r transcript.TaskRecord, parentPath string) (subagentRef, bool) {
	dir := strings.TrimSuffix(parentPath, ".jsonl") + "/subagents"
	ref := subagentRef{AgentID: r.ID}
	if ref.AgentID == "" && r.CallID != "" {
		ref.AgentID = c.subagentScans.agentFor(dir, r.CallID, c.scanSubagentDir)
	}
	switch {
	case safeShellOutputPath(r.OutputPath):
		ref.Path = r.OutputPath
	case validAgentID(ref.AgentID):
		ref.Path = dir + "/agent-" + ref.AgentID + ".jsonl"
	default:
		return subagentRef{}, false
	}
	if validAgentID(ref.AgentID) {
		compute := readSubagentMeta
		if m, err := c.subagentMetas.Read(dir+"/agent-"+ref.AgentID+".meta.json", compute); err == nil {
			ref.AgentType, ref.ParentAgentID, ref.Finished = m.AgentType, m.ParentAgentID, len(m.Finished) > 0 && string(m.Finished) != "null"
		}
	}
	return ref, true
}

// codexSubagent resolves a Codex call to its child rollout through the
// record's thread id. A call whose "started" event has not come, or whose
// rollout is missing, gives false.
func (c *Chat) codexSubagent(r transcript.TaskRecord, meta Meta) (subagentRef, bool) {
	if r.ID == "" {
		return subagentRef{}, false
	}
	path, ok := c.codexChildPath(r.ID, meta)
	if !ok {
		return subagentRef{}, false
	}
	return subagentRef{Path: path, AgentID: r.ID}, true
}
