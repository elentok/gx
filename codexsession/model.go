package codexsession

import (
	"bufio"
	"encoding/json"
	"os"
	"time"
)

// TurnModel is the model and reasoning effort of a rollout's newest turn.
type TurnModel struct {
	Model  string
	Effort string
	// At is the turn_context line's time, zero when it has none.
	At time.Time
}

// ReadTurnModel reads the newest `turn_context` line of the rollout at path.
// ok is false when the rollout has no turn yet.
func ReadTurnModel(path string) (m TurnModel, ok bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return TurnModel{}, false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var line struct {
			Type      string `json:"type"`
			Timestamp string `json:"timestamp"`
			Payload   struct {
				Model  string `json:"model"`
				Effort string `json:"effort"`
			} `json:"payload"`
		}
		if json.Unmarshal(scanner.Bytes(), &line) != nil || line.Type != "turn_context" || line.Payload.Model == "" {
			continue
		}
		at, _ := time.Parse(time.RFC3339Nano, line.Timestamp)
		m, ok = TurnModel{Model: line.Payload.Model, Effort: line.Payload.Effort, At: at}, true
	}
	if err := scanner.Err(); err != nil {
		return TurnModel{}, false, err
	}
	return m, ok, nil
}
