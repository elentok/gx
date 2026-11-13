package main

import (
	"os"
	"path/filepath"
)

// fakeNpmBody is the `npm ci` of the launching scenario: it marks that it
// started and waits while the npm-hold file exists, so the ticket stays at the
// installing-dependencies launch step until a test removes the hold.
const fakeNpmBody = `#!/bin/sh
dir=$(dirname "$0")/..
touch "$dir/npm-started"
while [ -e "$dir/npm-hold" ]; do sleep 0.05; done
`

// failingNpmBody is the `npm ci` of the launch-fail scenario: it exits at once
// with a message the pause reason carries.
const failingNpmBody = `#!/bin/sh
echo "npm ci: registry unreachable" >&2
exit 1
`

// setupFakeNpm writes <gate dir>/bin/npm and starts the hold on it.
func (f *fixture) setupFakeNpm(failing bool) error {
	body := fakeNpmBody
	if failing {
		body = failingNpmBody
	}
	if err := f.putBinFirst(map[string]string{"npm": body}); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(gateDir(f), "npm-hold"), nil, 0o644)
}

// configFile is the global config the server loaded; a test edits it and reloads.
func configFile(root string) string {
	return filepath.Join(root, "home", ".config", "gx", "config.json")
}
