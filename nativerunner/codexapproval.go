package nativerunner

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// The PermissionRequest hook command and the PTY runner talk through two files
// in the agent dir: the hook writes the request and polls for the decision,
// the runner reads the request for the card and writes the decision.
const (
	PermissionRequestFile  = "permission-request.json"
	PermissionDecisionFile = "permission-decision.json"
)

// PermissionRequest is what the hook saw. ID names this request, so a decision
// for an older one is never taken for the current one.
type PermissionRequest struct {
	ID   string `json:"id"`
	Tool string `json:"tool"`
	// Input is the tool's input as Codex gave it; Description is the command
	// of a shell tool, for the card's text.
	Input       json.RawMessage `json:"input,omitempty"`
	Description string          `json:"description,omitempty"`
}

// PermissionDecision answers the request with the same ID. Reason is the
// person's text on a deny, which the model sees.
type PermissionDecision struct {
	ID     string `json:"id"`
	Allow  bool   `json:"allow"`
	Reason string `json:"reason,omitempty"`
}

// ReadPermissionRequest returns the hook's pending request in dir; ok is false
// when there is none.
func ReadPermissionRequest(dir string) (req PermissionRequest, ok bool, err error) {
	ok, err = readJSON(filepath.Join(dir, PermissionRequestFile), &req)
	return req, ok && req.ID != "", err
}

// ReadPermissionDecision returns the decision in dir; ok is false when there
// is none yet.
func ReadPermissionDecision(dir string) (d PermissionDecision, ok bool, err error) {
	ok, err = readJSON(filepath.Join(dir, PermissionDecisionFile), &d)
	return d, ok, err
}

// WritePermissionRequest and WritePermissionDecision write atomically, so a
// poller never reads half a file.
func WritePermissionRequest(dir string, req PermissionRequest) error {
	return writeJSON(filepath.Join(dir, PermissionRequestFile), req)
}

func WritePermissionDecision(dir string, d PermissionDecision) error {
	return writeJSON(filepath.Join(dir, PermissionDecisionFile), d)
}

// ClearPermission removes both files.
func ClearPermission(dir string) {
	_ = os.Remove(filepath.Join(dir, PermissionRequestFile))
	_ = os.Remove(filepath.Join(dir, PermissionDecisionFile))
}

func readJSON(path string, v any) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(data, v)
}

func writeJSON(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}
