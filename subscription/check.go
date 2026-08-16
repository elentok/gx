// Package subscription implements a best-effort, once-per-process check of
// whether the operator's Claude account is configured to auto-purchase
// extra API credits once its subscription's included usage runs out.
package subscription

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// State is the detected extra-usage auto-purchase setting on the operator's
// Claude account.
type State int

const (
	// StateUnknown means the setting couldn't be verified: the account
	// state file is missing/unreadable, unparsable, or the field this
	// package looks for is missing or an unexpected shape.
	StateUnknown State = iota
	// StateEnabled means the account will auto-purchase extra usage.
	StateEnabled
	// StateDisabled means the account will not auto-purchase extra usage.
	StateDisabled
)

// accountFile mirrors the subset of Claude Code's own cached account state
// (~/.claude.json) this package reads. hasExtraUsageEnabled is an
// undocumented internal field, not a stable public schema, so every field
// here is a pointer: a missing or renamed field must resolve to
// StateUnknown, never a crash or a false StateDisabled.
type accountFile struct {
	OauthAccount *struct {
		HasExtraUsageEnabled *bool `json:"hasExtraUsageEnabled"`
	} `json:"oauthAccount"`
}

// Detect parses raw JSON bytes in Claude Code's cached account-state shape
// and returns the extra-usage state. Malformed JSON or a missing/renamed
// field resolves to StateUnknown.
func Detect(data []byte) State {
	var f accountFile
	if err := json.Unmarshal(data, &f); err != nil {
		return StateUnknown
	}
	if f.OauthAccount == nil || f.OauthAccount.HasExtraUsageEnabled == nil {
		return StateUnknown
	}
	if *f.OauthAccount.HasExtraUsageEnabled {
		return StateEnabled
	}
	return StateDisabled
}

var (
	checkOnce  sync.Once
	checkState State
)

// Check returns the extra-usage state, reading Claude Code's cached account
// state file once per gx process and caching the result for the process's
// lifetime.
func Check() State {
	checkOnce.Do(func() {
		checkState = detectFromDefaultPath()
	})
	return checkState
}

func detectFromDefaultPath() State {
	home, err := os.UserHomeDir()
	if err != nil {
		return StateUnknown
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		return StateUnknown
	}
	return Detect(data)
}
