package git

import (
	"testing"
)

func TestExtractPRURL(t *testing.T) {
	// Simulated GitHub stderr from a first-time push
	githubOutput := `
remote: Create a pull request for 'my-branch' on GitHub by visiting:
remote:      https://github.com/elentok/gx/pull/new/my-branch
remote:
`
	got := ExtractPRURL(githubOutput)
	want := "https://github.com/elentok/gx/pull/new/my-branch"
	if got != want {
		t.Fatalf("ExtractPRURL() = %q, want %q", got, want)
	}

	if got := ExtractPRURL("remote: Everything up-to-date\n"); got != "" {
		t.Fatalf("ExtractPRURL() = %q, want empty", got)
	}
}

func TestExtractPRURL_StripsTerminalEscapes(t *testing.T) {
	const want = "https://github.com/elentok/gx/pull/new/my-branch"
	output := "" +
		"remote: Create a pull request for 'my-branch' on GitHub by visiting:\n" +
		"remote: \x1b[32m\x1b]8;;" + want + "\x07" + want + "\x1b]8;;\x07\x1b[0m\n"

	if got := ExtractPRURL(output); got != want {
		t.Fatalf("ExtractPRURL() = %q, want %q", got, want)
	}
}

func TestIsNonFastForwardPushError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "non fast forward",
			err: &RunError{
				Stderr: "! [rejected]        main -> main (non-fast-forward)\nerror: failed to push some refs",
			},
			want: true,
		},
		{
			name: "fetch first",
			err: &RunError{
				Stderr: "Updates were rejected because the remote contains work that you do not have locally. (fetch first)",
			},
			want: true,
		},
		{
			name: "other error",
			err: &RunError{
				Stderr: "fatal: could not read from remote repository",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNonFastForwardPushError(tt.err); got != tt.want {
				t.Fatalf("IsNonFastForwardPushError() = %v, want %v", got, tt.want)
			}
		})
	}
}
