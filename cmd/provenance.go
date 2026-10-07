package cmd

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/elentok/gx/repair"
)

// Every --json result says how the write was made (via) and by whom (actor).
const (
	viaDirect = "direct"
	viaServer = "server"

	actorHuman    = "human"
	actorAgent    = "agent"
	actorRecovery = "recovery"
)

// callerActor is agent on a ralph-loop/* branch (an iteration's worktree) and
// human anywhere else, including when the branch can't be read. Recovery verbs
// don't use it: they are always actorRecovery.
func callerActor(getwd func() (string, error)) string {
	if _, ok := repair.IsRalphLoopBranch(getwd); ok {
		return actorAgent
	}
	return actorHuman
}

// stampProvenance adds via and actor to v's JSON object, whatever struct v is,
// so each result type needn't carry the two fields itself.
func stampProvenance(v any, via, actor string) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	out["via"] = via
	out["actor"] = actor
	return out, nil
}

// encodeProvenance writes v as indented JSON, stamped with via and actor when
// it is a JSON object. A JSON array is written as is: stamping it would mean
// wrapping it, which breaks every reader that expects the array.
func encodeProvenance(w io.Writer, v any, via, actor string) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	out := any(v)
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		if out, err = stampProvenance(v, via, actor); err != nil {
			return err
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
