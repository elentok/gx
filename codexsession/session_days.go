package codexsession

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DayUsage is the tokens one Codex session used on one local day.
type DayUsage struct {
	SessionID string
	Cwd       string
	// Day is the local calendar day, formatted "2006-01-02".
	Day    string
	Tokens int
}

// SessionDaysIn reads every rollout under home/sessions and returns each
// session's token delta per local day (day boundaries taken in loc). Deltas
// come from the cumulative total_tokens of token_count events, so they sum to
// the same total readStats reports. A torn line is skipped.
func SessionDaysIn(home string, loc *time.Location) ([]DayUsage, error) {
	var out []DayUsage
	err := filepath.WalkDir(filepath.Join(home, "sessions"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "rollout-") || !strings.HasSuffix(name, ".jsonl") {
			return nil
		}
		days, err := readSessionDays(path, loc)
		out = append(out, days...)
		return err
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return out, err
}

func readSessionDays(path string, loc *time.Location) ([]DayUsage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	type key struct{ id, day string }
	var (
		id, cwd string
		prev    int
		sums    = map[key]int{}
		cwds    = map[string]string{}
	)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var line sessionLine
		if json.Unmarshal(scanner.Bytes(), &line) != nil {
			continue
		}
		if line.Type == "session_meta" {
			id, cwd = line.Payload.ID, line.Payload.Cwd
			continue
		}
		total := line.Payload.Info.TotalTokenUsage.TotalTokens
		if id == "" || line.Type != "event_msg" || line.Payload.Type != "token_count" || total <= 0 || line.Timestamp.IsZero() {
			continue
		}
		k := key{id, line.Timestamp.In(loc).Format(time.DateOnly)}
		sums[k] += total - prev
		cwds[id] = cwd
		prev = total
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	out := make([]DayUsage, 0, len(sums))
	for k, tokens := range sums {
		out = append(out, DayUsage{SessionID: k.id, Cwd: cwds[k.id], Day: k.day, Tokens: tokens})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day < out[j].Day })
	return out, nil
}
