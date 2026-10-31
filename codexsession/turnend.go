package codexsession

import (
	"bufio"
	"encoding/json"
	"os"
)

// TurnEnded reports whether the rollout at path ended its turn: a task_complete
// event follows the last visible (user or assistant) message.
func TurnEnded(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	ended := false
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var line rolloutLine
		if json.Unmarshal(scanner.Bytes(), &line) != nil {
			continue
		}
		switch {
		case line.Type == "response_item" && line.Payload.Type == "message":
			ended = false
		case line.Type == "event_msg" && line.Payload.Type == "task_complete":
			ended = true
		}
	}
	return ended, scanner.Err()
}
