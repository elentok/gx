package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/server/servertest"
)

// fakeGoBody is a `go` that is a process the cache gate's scan finds. `clean`
// logs and returns; any other command records its pid and waits while the
// go-hold file exists.
const fakeGoBody = `#!/bin/sh
dir=$(dirname "$0")/..
case "$1" in
clean)
	echo "go $*" >> "$dir/go-clean.log"
	exit 0
	;;
esac
echo $$ >> "$dir/go-pids"
touch "$dir/go-started"
while [ -e "$dir/go-hold" ]; do sleep 0.05; done
`

// fakePsBody is the ps the server and gx brief find first. A script shows up in
// the real ps under its interpreter's name, and the developer's own `go test`
// (the one running this fixture) would hold the gate, so it lists the real
// processes except every `go`, plus a `go` for each live pid in go-pids. Any
// other ps call (the health scenario's lstart lookups) goes to the real one.
const fakePsBody = `#!/bin/sh
dir=$(dirname "$0")/..
[ "$*" = "-axo comm=" ] || exec /bin/ps "$@"
/bin/ps -axo comm= | while read -r comm; do
	[ "${comm##*/}" = go ] || echo "$comm"
done
while read -r pid; do
	kill -0 "$pid" 2>/dev/null && echo go
done < "$dir/go-pids" 2>/dev/null
exit 0
`

// cacheGateEpics is the one epic that gives the disk a project to clean.
var cacheGateEpics = []epic{{project, "gate-epic", []ticket{
	{"01", "first", servertest.TicketOpts{}},
}}}

// The cache-gate scenarios put <gate dir>/bin first on the server's PATH. A
// flow does the same for the gx it starts.
var (
	// cacheGate cleans through the disk control files.
	cacheGate = scenario{epics: cacheGateEpics, diskLow: true, fakeGo: true}
	// cacheGateCustom has no project cleanup command, so a
	// --config disk-space.cleanup-command override is the command that runs.
	cacheGateCustom = scenario{epics: cacheGateEpics, fakeGo: true}
)

// gateDir sits outside the project repo, so the trunk stays clean.
func (f *fixture) gateDir() string { return filepath.Join(f.root, "gate") }

// setupFakeGo writes <gate dir>/bin/go and bin/ps and puts the bin first on
// the server's PATH. A flow holds a build by creating <gate dir>/go-hold and
// releases it by removing the file; go-started marks a held command and
// go-clean.log holds a `go clean` that ran.
func (f *fixture) setupFakeGo() error {
	bin := filepath.Join(f.gateDir(), "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return err
	}
	for name, body := range map[string]string{"go": fakeGoBody, "ps": fakePsBody} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			return err
		}
	}
	path := ""
	for _, kv := range f.env {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			path = v
		}
	}
	// The last PATH entry wins.
	f.env = append(f.env, "PATH="+bin+string(os.PathListSeparator)+path)
	return nil
}
