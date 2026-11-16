package config

import (
	"strings"
	"testing"
)

func TestMentalHealthDefaultsAndValidation(t *testing.T) {
	cfg, err := Parse([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	want := MentalHealthConfig{
		Bedtime: MentalHealthBedtime{Start: "22:00", End: "07:00", Message: "That's enough AI for today. Go to sleep and come back tomorrow at {end}."},
		Break:   MentalHealthBreak{WorkMinutes: 55, BreakMinutes: 5},
	}
	if cfg.MentalHealth != want {
		t.Fatalf("defaults = %+v, want %+v", cfg.MentalHealth, want)
	}

	// Values that fail their check keep the default, like other numeric keys.
	cfg, err = Parse([]byte(`{"mental-health":{"enabled":true,"bedtime":{"start":"23:30","end":"7am"},"break":{"work-minutes":2,"break-minutes":0}}}`))
	if err != nil {
		t.Fatal(err)
	}
	m := cfg.MentalHealth
	if !m.Enabled || m.Bedtime.Start != "23:30" || m.Bedtime.End != "07:00" || m.Break.WorkMinutes != 55 || m.Break.BreakMinutes != 5 {
		t.Fatalf("overlay = %+v", m)
	}

	_, err = Parse([]byte(`{"mental-health":{"bedtime":{"start":"07:00"}}}`))
	if err == nil || !strings.Contains(err.Error(), "bedtime.start") || !strings.Contains(err.Error(), "bedtime.end") {
		t.Fatalf("equal start and end: err = %v", err)
	}
}
