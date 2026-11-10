package pr

import (
	"reflect"
	"testing"
)

func done(wf, name, sha, conclusion string) Check {
	return Check{WorkflowID: wf, Name: name, HeadSHA: sha, Status: "COMPLETED", Conclusion: conclusion}
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
			got, failed := ClassifyChecks("head", c.checks)
			if got != c.want {
				t.Errorf("result: expected %v, got %v", c.want, got)
			}
			if !reflect.DeepEqual(failed, c.failed) {
				t.Errorf("failed: expected %v, got %v", c.failed, failed)
			}
		})
	}
}
