package ui

import "strings"

// HarnessLabel is `harness · tier · model effort` as plain text, shared by the
// Agents and Queue tabs. Before a reply the model reads "CLI default".
func HarnessLabel(harness, tier, model, effort string, switched bool) string {
	parts := []string{harnessName(harness)}
	if t := tierName(tier); t != "" {
		parts = append(parts, t)
	}
	if model == "" {
		parts = append(parts, "CLI default")
	} else {
		parts = append(parts, strings.TrimSpace(model+" "+effort))
	}
	label := strings.Join(parts, " · ")
	if switched {
		label += " · switched by hand"
	}
	return label
}

// QueueHarnessLabel is a Queue row's text: the full label for a run, or for a
// waiting ticket its note, led by the harness only when the note does not
// already name it. Empty when the ticket has no label.
func QueueHarnessLabel(harness, tier, model, effort, note string) string {
	switch {
	case note == "" && harness == "":
		return ""
	case note == "":
		return HarnessLabel(harness, tier, model, effort, false)
	case harness == "" || strings.HasPrefix(note, "keeps "):
		return note
	}
	return harnessName(harness) + " · " + note
}

func harnessName(h string) string {
	if h == "" {
		return ""
	}
	return strings.ToUpper(h[:1]) + h[1:]
}

func tierName(t string) string {
	switch t {
	case "light":
		return "Light"
	case "worker":
		return "Worker"
	case "strongest":
		return "Strongest"
	}
	return ""
}
