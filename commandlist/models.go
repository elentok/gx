package commandlist

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Model is one row of a harness's model list, as the harness sent it.
type Model struct {
	// Value is what a launch takes (Codex id, Claude value).
	Value string
	// ID is the resolved model id (Codex model, Claude resolvedModel).
	ID string
	// Hidden is the Codex hidden flag; always false for Claude.
	Hidden bool
	// Efforts are the supported efforts in the harness's order; empty when the
	// model has none.
	Efforts []string
	// DefaultEffort is the Codex default effort; empty for Claude.
	DefaultEffort string
	// Upgrade is the successor the harness names; empty when it names none.
	Upgrade string
	// IsDefault is the Codex isDefault flag: the model a launch with no model uses.
	IsDefault bool
}

type claudeModelsReply struct {
	Type     string `json:"type"`
	Response struct {
		Response struct {
			Models *[]struct {
				Value                 string   `json:"value"`
				ResolvedModel         string   `json:"resolvedModel"`
				SupportedEffortLevels []string `json:"supportedEffortLevels"`
			} `json:"models"`
		} `json:"response"`
	} `json:"response"`
}

// ProbeClaudeModels lists the models a Claude session started in dir would
// offer. It reads the models of the one initialize reply: no model call is
// made and no transcript is left. A row without a resolved model is left out.
func ProbeClaudeModels(ctx context.Context, dir string, env []string) ([]Model, error) {
	return runClaudeInitialize(ctx, dir, env, claudeProbeTimeout, parseClaudeModels)
}

func parseClaudeModels(r io.Reader) ([]Model, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var msg claudeModelsReply
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			return nil, fmt.Errorf("claude probe: bad JSON line: %w", err)
		}
		if msg.Type != "control_response" {
			continue
		}
		rows := msg.Response.Response.Models
		if rows == nil {
			return nil, errors.New("claude probe: reply has no models")
		}
		out := make([]Model, 0, len(*rows))
		for _, m := range *rows {
			if m.ResolvedModel == "" {
				continue
			}
			efforts := m.SupportedEffortLevels
			if efforts == nil {
				efforts = []string{}
			}
			out = append(out, Model{Value: m.Value, ID: m.ResolvedModel, Efforts: efforts})
		}
		return out, nil
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("claude probe: read reply: %w", err)
	}
	return nil, errNoReply
}

type codexModelsReply struct {
	ID     *int `json:"id"`
	Result struct {
		Data []struct {
			ID                        string `json:"id"`
			Model                     string `json:"model"`
			Hidden                    bool   `json:"hidden"`
			IsDefault                 bool   `json:"isDefault"`
			DefaultReasoningEffort    string `json:"defaultReasoningEffort"`
			SupportedReasoningEfforts []struct {
				ReasoningEffort string `json:"reasoningEffort"`
			} `json:"supportedReasoningEfforts"`
			Upgrade     *string `json:"upgrade"`
			UpgradeInfo *struct {
				Model string `json:"model"`
			} `json:"upgradeInfo"`
		} `json:"data"`
		NextCursor *string `json:"nextCursor"`
	} `json:"result"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ProbeCodexModels lists the models `codex app-server` (run in dir with env)
// offers, hidden ones included, following nextCursor to the last page.
func ProbeCodexModels(ctx context.Context, dir string, env []string) ([]Model, error) {
	return runCodexAppServer(ctx, dir, env, func(send func(any) error, stdout io.Reader) ([]Model, error) {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		var out []Model
		var cursor *string
		// Request ids 1 is the handshake; each page takes the next id.
		for id := 2; ; id++ {
			params := map[string]any{"includeHidden": true}
			if cursor != nil {
				params["cursor"] = *cursor
			}
			if err := send(map[string]any{"jsonrpc": "2.0", "id": id, "method": "model/list", "params": params}); err != nil {
				return nil, err
			}
			reply, err := readCodexModelsReply(sc, id)
			if err != nil {
				return nil, err
			}
			for _, m := range reply.Result.Data {
				row := Model{Value: m.ID, ID: m.Model, Hidden: m.Hidden, IsDefault: m.IsDefault, DefaultEffort: m.DefaultReasoningEffort, Efforts: []string{}}
				for _, e := range m.SupportedReasoningEfforts {
					row.Efforts = append(row.Efforts, e.ReasoningEffort)
				}
				switch {
				case m.Upgrade != nil:
					row.Upgrade = *m.Upgrade
				case m.UpgradeInfo != nil:
					row.Upgrade = m.UpgradeInfo.Model
				}
				out = append(out, row)
			}
			cursor = reply.Result.NextCursor
			if cursor == nil || *cursor == "" {
				if out == nil {
					out = []Model{}
				}
				return out, nil
			}
		}
	})
}

func readCodexModelsReply(sc *bufio.Scanner, id int) (codexModelsReply, error) {
	for sc.Scan() {
		var reply codexModelsReply
		if json.Unmarshal(sc.Bytes(), &reply) != nil || reply.ID == nil || *reply.ID != id {
			continue
		}
		if reply.Error != nil {
			return reply, errors.New("codex model/list: " + reply.Error.Message)
		}
		return reply, nil
	}
	if err := sc.Err(); err != nil {
		return codexModelsReply{}, err
	}
	return codexModelsReply{}, errors.New("codex app-server closed without a model/list reply")
}
