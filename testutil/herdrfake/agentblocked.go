package herdrfake

import (
	"encoding/json"
	"fmt"
)

// AgentBlockedError builds the herdr 0.8.2 agent_blocked JSON error envelope
// as a Go error, so a fake's CommandFunc can return it directly: the
// herdrfake dispatch adapter turns a returned error into the command's
// stdout via its Error() string, and that string must be exactly what
// herdr.run() parses into an *herdr.AgentBlockedError. Written once here so
// every fake that models a blocked pane emits the identical wire shape.
func AgentBlockedError(target string) error {
	envelope := struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{}
	envelope.Error.Code = "agent_blocked"
	envelope.Error.Message = fmt.Sprintf("agent %s is blocked and not accepting prompts", target)
	b, err := json.Marshal(envelope)
	if err != nil {
		panic(err)
	}
	return errorString(b)
}

// errorString is a []byte whose Error() is its own content, letting
// AgentBlockedError return an error without an extra allocation-hiding
// fmt.Errorf indirection.
type errorString []byte

func (e errorString) Error() string { return string(e) }
