package codexsession

import (
	"bufio"
	"encoding/json"
	"os"
	"time"
)

// ModelNotFound is the error kind of a turn on a model that does not exist or
// is not available to the account. Codex does not exit on it.
const ModelNotFound = "model_not_found"

// FirstTurnEndAfter reads the rollout at path for the first task_complete
// stamped after at. found is false while there is none, a turn still running
// included. kind is the turn's error kind, "" for a clean completion. Lines
// with no time never count.
func FirstTurnEndAfter(path string, at time.Time) (kind string, found bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var line sessionLine
		if json.Unmarshal(scanner.Bytes(), &line) != nil || line.Type != "event_msg" || line.Payload.Type != "task_complete" {
			continue
		}
		if !line.Timestamp.After(at) {
			continue
		}
		return errorKind(line.Payload.Error.Info), true, nil
	}
	return "", false, scanner.Err()
}
