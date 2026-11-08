package tickets

import (
	"slices"
	"testing"
)

func TestFlowSpec_Design(t *testing.T) {
	t.Parallel()
	spec, err := FlowSpecFor(FlowDesign)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(spec.Steps, DesignStepNames()) || len(spec.Steps) != 8 {
		t.Fatalf("steps = %v", spec.Steps)
	}
	for _, s := range spec.Steps {
		if spec.Labels[s] == "" {
			t.Errorf("no label for %q", s)
		}
	}
	if !spec.IsFinal("branch") || !spec.IsFinal("done") || spec.IsFinal("spec") {
		t.Errorf("final = %v", spec.Final)
	}
	if spec.StepNeedsPerson("spec") {
		t.Error("design has no needs-person steps")
	}
	if spec.Skill != "gx-design" || spec.ContinueArg("gx:m") != "gx:m" {
		t.Errorf("skill %q arg %q", spec.Skill, spec.ContinueArg("gx:m"))
	}
}

func TestFlowSpec_Triage(t *testing.T) {
	t.Parallel()
	spec, err := FlowSpecFor(FlowTriage)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"verify", "plan", "approve", "write", "done"}
	if !slices.Equal(spec.Steps, want) {
		t.Fatalf("steps = %v", spec.Steps)
	}
	for _, s := range want {
		if spec.Labels[s] == "" {
			t.Errorf("no label for %q", s)
		}
	}
	if !spec.IsFinal("done") || spec.IsFinal("write") {
		t.Errorf("final = %v", spec.Final)
	}
	if !spec.StepNeedsPerson("plan") || !spec.StepNeedsPerson("approve") || spec.StepNeedsPerson("verify") {
		t.Errorf("needs person = %v", spec.NeedsPerson)
	}
	if spec.Skill != "gx-triage-follow-ups" || spec.ContinueArg("gx") != "--project gx" {
		t.Errorf("skill %q arg %q", spec.Skill, spec.ContinueArg("gx"))
	}
}

func TestFlow_RefusesUnknown(t *testing.T) {
	t.Parallel()
	if _, err := FlowSpecFor("nope"); err == nil {
		t.Error("unknown flow accepted")
	}
	if err := CheckFlowStep(FlowTriage, "chart"); err == nil {
		t.Error("design step accepted by triage")
	}
	if err := CheckFlowStep(FlowDesign, "verify"); err == nil {
		t.Error("triage step accepted by design")
	}
	if err := CheckFlowStep(FlowTriage, "approve"); err != nil {
		t.Error(err)
	}
	if err := CheckFlowStep("nope", "done"); err == nil {
		t.Error("unknown flow step accepted")
	}
}
