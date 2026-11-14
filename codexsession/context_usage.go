package codexsession

import (
	"bufio"
	"encoding/json"
	"os"
)

// TokenBreakdown is Codex's reported usage. Cached input and cache writes are
// subsets of input, and reasoning is a subset of output. Never add them twice.
type TokenBreakdown struct {
	Input     *int64 `json:"input_tokens"`
	Cached    *int64 `json:"cached_input_tokens"`
	Written   *int64 `json:"cache_write_input_tokens"`
	Output    *int64 `json:"output_tokens"`
	Reasoning *int64 `json:"reasoning_output_tokens"`
	Total     int64  `json:"total_tokens"`
}

// ContextUsage describes only the selected rollout. It does not sum child threads.
type ContextUsage struct {
	First, Peak, Compactions int
	Stale                    bool
	Capacity                 int
	Latest, Total            TokenBreakdown
}

// ReadContextUsage reads cumulative snapshots, never sums token_count events.
// gx measures request input, which differs from Codex's input+output context display.
func ReadContextUsage(path string) (ContextUsage, error) {
	f, err := os.Open(path)
	if err != nil {
		return ContextUsage{}, err
	}
	defer f.Close()
	var result ContextUsage
	var previous string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var line struct {
			Type    string `json:"type"`
			Payload struct {
				Type string `json:"type"`
				Info *struct {
					Latest   TokenBreakdown `json:"last_token_usage"`
					Total    TokenBreakdown `json:"total_token_usage"`
					Capacity int            `json:"model_context_window"`
				} `json:"info"`
			} `json:"payload"`
		}
		if json.Unmarshal(scanner.Bytes(), &line) != nil {
			continue
		}
		if line.Type == "compacted" {
			result.Compactions++
			result.Stale = true
			continue
		}
		info := line.Payload.Info
		if line.Type != "event_msg" || line.Payload.Type != "token_count" || info == nil {
			continue
		}
		if info.Capacity > 0 {
			result.Capacity = info.Capacity
		}
		if info.Total.Total > 0 {
			result.Total = info.Total
		}
		// Codex can emit a local estimate with input=0 after compaction. Keep the
		// last request reading and its stale marker until a fresh request arrives.
		if info.Latest.Input == nil || *info.Latest.Input <= 0 {
			continue
		}
		// Spending-only updates can change totals while retaining the old request.
		// They must not clear a compaction's stale marker.
		signature, _ := json.Marshal(info.Latest)
		if string(signature) == previous {
			continue
		}
		previous = string(signature)
		input := int(*info.Latest.Input)
		if result.First == 0 {
			result.First = input
		}
		result.Peak = max(result.Peak, input)
		result.Latest = info.Latest
		result.Stale = false
	}
	return result, scanner.Err()
}
