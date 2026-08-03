package tickets

import (
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/tickets"
)

func TestPreviewContent_Live_IncludesMetricsLineBeforeRule(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{{Identifier: "01", Title: "Running ticket", Status: "open"}}}
	m := newFlatModelForRowTests(epic)
	m.selected = 0
	m.live["01"] = liveTicketState{
		running: true, label: "iter-01",
		startedAt: time.Now().Add(-754 * time.Second), tokens: 45200,
	}

	content := m.previewContent(80)
	lines := strings.Split(content, "\n")

	metricsIdx := -1
	ruleIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "12m34s") && strings.Contains(line, "45.2k tok") {
			metricsIdx = i
		}
		if strings.Contains(line, "───") {
			ruleIdx = i
		}
	}
	if metricsIdx == -1 {
		t.Fatalf("expected a metrics line, got %#v", lines)
	}
	if ruleIdx == -1 || metricsIdx >= ruleIdx {
		t.Errorf("expected the metrics line before the rule, metricsIdx=%d ruleIdx=%d: %#v", metricsIdx, ruleIdx, lines)
	}
}

func TestPreviewContent_Done_IncludesMetricsLineBeforeRule(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Done ticket", Status: "done", ElapsedTime: 754, ActualContextWindow: 45200},
	}}
	m := newFlatModelForRowTests(epic)
	m.selected = 0

	content := m.previewContent(80)
	lines := strings.Split(content, "\n")

	metricsIdx := -1
	ruleIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "12m34s") && strings.Contains(line, "45.2k tok") {
			metricsIdx = i
		}
		if strings.Contains(line, "───") {
			ruleIdx = i
		}
	}
	if metricsIdx == -1 {
		t.Fatalf("expected a metrics line, got %#v", lines)
	}
	if ruleIdx == -1 || metricsIdx >= ruleIdx {
		t.Errorf("expected the metrics line before the rule, metricsIdx=%d ruleIdx=%d: %#v", metricsIdx, ruleIdx, lines)
	}
}

func TestPreviewContent_NeverRun_OmitsMetricsLine(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{{Identifier: "01", Title: "Open ticket", Status: "open"}}}
	m := newFlatModelForRowTests(epic)
	m.selected = 0

	content := m.previewContent(80)
	if strings.Contains(content, " tok") {
		t.Errorf("expected no metrics line for a never-run ticket, got %q", content)
	}
}
