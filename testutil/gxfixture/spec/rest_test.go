package spec

import (
	"regexp"
	"slices"
	"testing"
)

func TestRestScenario_OverridesComeFromTheShapeConfig(t *testing.T) {
	t.Parallel()
	breakKeys := []string{
		"mental-health.enabled=true",
		"mental-health.break.enabled=true",
		"mental-health.break.work-minutes=55",
		"mental-health.break.break-minutes=5",
	}
	for _, shape := range []string{"break", "break-warning"} {
		sc, ok := RestScenario(shape)
		if !ok || !slices.Equal(sc.Config, breakKeys) {
			t.Errorf("%s: Config = %q, want %q", shape, sc.Config, breakKeys)
		}
	}
	bedtime := []*regexp.Regexp{
		regexp.MustCompile(`^mental-health\.enabled=true$`),
		regexp.MustCompile(`^mental-health\.bedtime\.enabled=true$`),
		regexp.MustCompile(`^mental-health\.bedtime\.start="\d\d:\d\d"$`),
		regexp.MustCompile(`^mental-health\.bedtime\.end="\d\d:\d\d"$`),
	}
	for _, shape := range []string{"bedtime", "bedtime-warning"} {
		sc, ok := RestScenario(shape)
		if !ok || len(sc.Config) != len(bedtime) {
			t.Fatalf("%s: Config = %q, want %d keys", shape, sc.Config, len(bedtime))
		}
		for i, re := range bedtime {
			if !re.MatchString(sc.Config[i]) {
				t.Errorf("%s: Config[%d] = %q, want match %s", shape, i, sc.Config[i], re)
			}
		}
	}
}
