package codexsession

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
)

// ExportMarkdownIn renders the rollout of sessionID under the Codex home as
// Markdown holding the user and assistant message text only; tool calls and
// their output are left out. ok is false when no rollout exists.
func ExportMarkdownIn(home, sessionID string) (md string, ok bool, err error) {
	if sessionID == "" {
		return "", false, nil
	}
	err = walkSessionFiles(home, sessionID, func(path string) error {
		if ok {
			return nil
		}
		md, err = exportRollout(path)
		ok = err == nil
		return err
	})
	if err != nil {
		return "", false, err
	}
	return md, ok, nil
}

// LastAssistantText is LastAssistantTextIn under the user's Codex home.
func LastAssistantText(sessionID string) (text string, ok bool, err error) {
	home, err := codexHome()
	if err != nil {
		return "", false, err
	}
	return LastAssistantTextIn(home, sessionID)
}

// LastAssistantTextIn returns the text of the last assistant message in the
// rollout of sessionID under the Codex home. ok is false when no rollout
// exists or it holds no assistant text.
func LastAssistantTextIn(home, sessionID string) (text string, ok bool, err error) {
	if sessionID == "" {
		return "", false, nil
	}
	err = walkSessionFiles(home, sessionID, func(path string) error {
		if ok {
			return nil
		}
		text, ok, err = lastAssistantInRollout(path)
		return err
	})
	if err != nil {
		return "", false, err
	}
	return text, ok, nil
}

func lastAssistantInRollout(path string) (text string, ok bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var line rolloutLine
		if json.Unmarshal(scanner.Bytes(), &line) != nil {
			continue
		}
		if line.Type != "response_item" || line.Payload.Type != "message" || line.Payload.Role != "assistant" {
			continue
		}
		var texts []string
		for _, c := range line.Payload.Content {
			if t := strings.TrimSpace(c.Text); t != "" {
				texts = append(texts, t)
			}
		}
		if len(texts) > 0 {
			text, ok = strings.Join(texts, "\n\n"), true
		}
	}
	return text, ok, scanner.Err()
}

type rolloutLine struct {
	Type    string `json:"type"`
	Payload struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"payload"`
}

func exportRollout(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var parts []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var line rolloutLine
		if json.Unmarshal(scanner.Bytes(), &line) != nil {
			continue
		}
		if line.Type != "response_item" || line.Payload.Type != "message" {
			continue
		}
		var heading string
		switch line.Payload.Role {
		case "user":
			heading = "## User"
		case "assistant":
			heading = "## Assistant"
		default:
			continue
		}
		var texts []string
		for _, c := range line.Payload.Content {
			if t := strings.TrimSpace(c.Text); t != "" {
				texts = append(texts, t)
			}
		}
		if len(texts) > 0 {
			parts = append(parts, heading+"\n\n"+strings.Join(texts, "\n\n"))
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if len(parts) == 0 {
		return "", nil
	}
	return strings.Join(parts, "\n\n---\n\n") + "\n", nil
}
