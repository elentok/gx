package commandlist

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// codexProbeTimeout bounds the whole app-server exchange.
var codexProbeTimeout = 15 * time.Second

const codexSkillsRequestID = 2

type codexSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type codexReply struct {
	ID     *int `json:"id"`
	Result struct {
		Data []struct {
			Skills []codexSkill `json:"skills"`
		} `json:"data"`
	} `json:"result"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ProbeCodexSkills asks `codex app-server` (run in dir with env) for the skills
// it would offer after "$". Per-file load errors in the reply are ignored.
func ProbeCodexSkills(ctx context.Context, dir string, env []string) ([]Entry, error) {
	ctx, cancel := context.WithTimeout(ctx, codexProbeTimeout)
	defer cancel()

	cmd := commandIn(ctx, env, "codex", "app-server")
	cmd.Dir = dir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex app-server: %w", err)
	}
	// The child is killed once the reply is read (or the context ends).
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()

	requests := []any{
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize",
			"params": map[string]any{"clientInfo": map[string]any{"name": "gx", "version": "0"}}},
		map[string]any{"jsonrpc": "2.0", "method": "initialized"},
		map[string]any{"jsonrpc": "2.0", "id": codexSkillsRequestID, "method": "skills/list",
			"params": map[string]any{"cwds": []string{dir}, "forceReload": true}},
	}
	enc := json.NewEncoder(stdin)
	for _, r := range requests {
		if err := enc.Encode(r); err != nil {
			return nil, fmt.Errorf("write to codex app-server: %w", err)
		}
	}

	entries, err := parseCodexSkills(stdout)
	if err != nil && ctx.Err() != nil {
		return nil, fmt.Errorf("codex app-server timed out: %w", ctx.Err())
	}
	return entries, err
}

// parseCodexSkills reads JSON lines until the skills/list reply and returns its
// enabled skills, each name once.
func parseCodexSkills(r io.Reader) ([]Entry, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var reply codexReply
		if json.Unmarshal(sc.Bytes(), &reply) != nil || reply.ID == nil || *reply.ID != codexSkillsRequestID {
			continue
		}
		if reply.Error != nil {
			return nil, errors.New("codex skills/list: " + reply.Error.Message)
		}
		seen := map[string]bool{}
		entries := []Entry{}
		for _, cwd := range reply.Result.Data {
			for _, s := range cwd.Skills {
				if !s.Enabled || seen[s.Name] {
					continue
				}
				seen[s.Name] = true
				entries = append(entries, Entry{Name: s.Name, Description: s.Description, Source: SourceSkill})
			}
		}
		return entries, nil
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("codex app-server closed without a skills/list reply")
}
