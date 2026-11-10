package codexsession

import (
	"encoding/json"
	"os"

	"github.com/elentok/gx/transcript"
)

// QuestionToolName is the Codex tool that asks the person questions.
const QuestionToolName = "request_user_input"

// OpenQuestionOption is one choice of a question the tool asks.
type OpenQuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// OpenQuestionItem is one question of a request_user_input call.
type OpenQuestionItem struct {
	ID       string               `json:"id"`
	Header   string               `json:"header"`
	Question string               `json:"question"`
	Options  []OpenQuestionOption `json:"options"`
}

// OpenQuestion is a request_user_input call that has no output yet.
type OpenQuestion struct {
	CallID    string
	Questions []OpenQuestionItem
}

// ReadOpenQuestion returns the newest open request_user_input call of the
// rollout's current turn (the turn starts at the last task_started event), or
// ok false when none is open. A call whose arguments do not parse is skipped.
// A missing file has nothing open.
func ReadOpenQuestion(path string) (q OpenQuestion, ok bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return OpenQuestion{}, false, nil
		}
		return OpenQuestion{}, false, err
	}
	defer f.Close()

	err = transcript.ForEachLine(f, func(raw []byte) {
		var line chatRolloutLine
		if json.Unmarshal(raw, &line) != nil {
			return
		}
		p := line.Payload
		switch {
		case line.Type == "event_msg" && p.Type == "task_started":
			q, ok = OpenQuestion{}, false
		case line.Type != "response_item" || p.CallID == "":
		case p.Type == "function_call" && p.Name == QuestionToolName:
			var args struct {
				Questions []OpenQuestionItem `json:"questions"`
			}
			if json.Unmarshal([]byte(p.Arguments), &args) == nil && len(args.Questions) > 0 {
				q, ok = OpenQuestion{CallID: p.CallID, Questions: args.Questions}, true
			}
		case p.Type == "function_call_output" && ok && p.CallID == q.CallID:
			q, ok = OpenQuestion{}, false
		}
	})
	if err != nil {
		return OpenQuestion{}, false, err
	}
	return q, ok, nil
}
