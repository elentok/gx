package pr

import (
	"reflect"
	"testing"
)

func done(wf, name, sha, conclusion string) Check {
	return Check{WorkflowID: wf, Name: name, HeadSHA: sha, Status: "COMPLETED", Conclusion: conclusion}
}

func TestCountChecks(t *testing.T) {
	checks := []Check{
		done("1", "a", "head", "SUCCESS"), done("1", "b", "head", "FAILURE"),
		{Name: "c", HeadSHA: "head", Status: "IN_PROGRESS"}, done("1", "d", "old", "FAILURE"),
		done("1", "b", "head", "SUCCESS"), // a rerun replaces the earlier b
	}
	if got, want := Summarize("head", checks).Counts, (CheckCounts{Pending: 1, Green: 2}); got != want {
		t.Fatalf("counts = %+v, want %+v", got, want)
	}
}

func TestClassifyChecks(t *testing.T) {
	cases := []struct {
		name   string
		checks []Check
		want   ChecksResult
		failed []CheckKey
	}{
		{"none", nil, ChecksNone, nil},
		{"older head ignored", []Check{done("1", "a", "old", "FAILURE")}, ChecksNone, nil},
		{"older head failure does not fail", []Check{
			done("1", "a", "old", "FAILURE"), done("1", "a", "head", "SUCCESS"),
		}, ChecksGreen, nil},
		{"green needs success neutral skipped", []Check{
			done("1", "a", "head", "SUCCESS"), done("1", "b", "head", "NEUTRAL"), done("1", "c", "head", "SKIPPED"),
		}, ChecksGreen, nil},
		{"cancelled is pending", []Check{done("1", "a", "head", "CANCELLED")}, ChecksPending, nil},
		{"stale is pending", []Check{done("1", "a", "head", "STALE")}, ChecksPending, nil},
		{"action required is pending", []Check{done("1", "a", "head", "ACTION_REQUIRED")}, ChecksPending, nil},
		{"running is pending", []Check{{Name: "a", HeadSHA: "head", Status: "IN_PROGRESS"}}, ChecksPending, nil},
		{"failed wins over pending", []Check{
			{Name: "a", HeadSHA: "head", Status: "QUEUED"}, done("1", "b", "head", "TIMED_OUT"),
		}, ChecksFailed, []CheckKey{{"1", "b"}}},
		{"startup failure fails", []Check{done("1", "a", "head", "STARTUP_FAILURE")}, ChecksFailed, []CheckKey{{"1", "a"}}},
		{"same name in two workflows are distinct", []Check{
			done("1", "build", "head", "FAILURE"), done("2", "build", "head", "SUCCESS"),
		}, ChecksFailed, []CheckKey{{"1", "build"}}},
		{"rerun of same key replaces earlier", []Check{
			done("1", "a", "head", "FAILURE"), done("1", "a", "head", "SUCCESS"),
		}, ChecksGreen, nil},
		{"status context keyed by context", []Check{done("", "ci/legacy", "head", "FAILURE")}, ChecksFailed, []CheckKey{{"", "ci/legacy"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Summarize("head", c.checks)
			if s.Result != c.want {
				t.Errorf("result: expected %v, got %v", c.want, s.Result)
			}
			if !reflect.DeepEqual(s.Failed, c.failed) {
				t.Errorf("failed: expected %v, got %v", c.failed, s.Failed)
			}
			if len(s.FailedChecks) != len(c.failed) {
				t.Fatalf("failed checks: expected %d, got %v", len(c.failed), s.FailedChecks)
			}
			for i, k := range c.failed {
				if s.FailedChecks[i].Key() != k {
					t.Errorf("failed check %d: expected key %v, got %v", i, k, s.FailedChecks[i].Key())
				}
			}
		})
	}
}
