package herdrfake

import (
	"fmt"
	"strings"
	"testing"
)

// recorder is a testing.TB whose Errorf records instead of failing, so a test
// can watch a forbidden fake report without failing itself.
type recorder struct {
	testing.TB
	errors []string
}

func (r *recorder) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func TestForbid_CallFailsTheTestNamingTheCommandAndAnswersWithAnError(t *testing.T) {
	rec := &recorder{TB: t}
	s := NewState(rec)
	s.Register("agent", "get", func(*State, []string) (any, Identities, error) { return map[string]any{}, Identities{}, nil })
	s.Forbid()

	out, code := s.Handler()([]string{"agent", "get", "p1"})

	if code == 0 {
		t.Errorf("exit code = 0, want an error answer; output %s", out)
	}
	if len(rec.errors) != 1 || !strings.Contains(rec.errors[0], "herdr agent get") {
		t.Errorf("reported errors = %q, want one naming %q", rec.errors, "herdr agent get")
	}
}

func TestForbid_OffByDefault(t *testing.T) {
	rec := &recorder{TB: t}
	s := NewState(rec)
	s.Register("agent", "get", func(*State, []string) (any, Identities, error) { return map[string]any{}, Identities{}, nil })

	_, code := s.Handler()([]string{"agent", "get", "p1"})

	if code != 0 || len(rec.errors) != 0 {
		t.Errorf("code = %d, errors = %q, want a plain success", code, rec.errors)
	}
}
