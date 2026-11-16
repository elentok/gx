package registry

import (
	"strings"
	"testing"
)

func TestMentalHealthValueChecks(t *testing.T) {
	reg := Default()
	cases := []struct {
		key, raw string
		ok       bool
	}{
		{"mental-health.bedtime.start", `"22:00"`, true},
		{"mental-health.bedtime.end", `"7:00"`, false},
		{"mental-health.bedtime.end", `"24:00"`, false},
		{"mental-health.bedtime.end", `"07:60"`, false},
		{"mental-health.bedtime.start", `"10pm"`, false},
		{"mental-health.break.work-minutes", `3`, true},
		{"mental-health.break.work-minutes", `2`, false},
		{"mental-health.break.work-minutes", `3.5`, false},
		{"mental-health.break.break-minutes", `1`, true},
		{"mental-health.break.break-minutes", `0`, false},
		{"mental-health.break.message", `""`, true},
	}
	for _, c := range cases {
		e, ok := reg.Lookup(Key(strings.Split(c.key, ".")...))
		if !ok {
			t.Fatalf("%s not in the registry", c.key)
		}
		if e.ProjectOverridable || e.ProjectOnly {
			t.Errorf("%s must be global-only", c.key)
		}
		if err := CheckValue(e, []byte(c.raw), nil); (err == nil) != c.ok {
			t.Errorf("%s = %s: err = %v, want ok=%v", c.key, c.raw, err, c.ok)
		}
	}
}
