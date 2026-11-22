package brief

import (
	"regexp"
	"strings"
)

var (
	checkSectionRE = regexp.MustCompile(`(?m)^== \S+ ==$`)
	slowLineRE     = regexp.MustCompile(`^\s*slow test (.+? took .+)$`)
	failedWordRE   = regexp.MustCompile(`(?mi)^FAIL\b|\b\d+ failed\b`)
)

// IsCheckLog reports whether data is a `gx check` test.log: the make-style
// run of several brief calls, each under a `== name ==` heading.
func IsCheckLog(data []byte) bool {
	return checkSectionRE.Match(data)
}

// SaysFailed reports whether a log says something failed, parsed or not.
func SaysFailed(data []byte) bool {
	return failedWordRE.Match(data)
}

// ParseCheckLog lists the failures a `gx check` test.log names: every test
// under a `FAIL tool:` section (go packages and playwright files share one
// shape: a location line, then the test name indented by two, then its text)
// and every `slow test` limit line.
func ParseCheckLog(data []byte) []Unit {
	var units []Unit
	inFail := false
	location := ""
	for line := range strings.Lines(string(data)) {
		line = strings.TrimRight(line, "\r\n")
		if m := slowLineRE.FindStringSubmatch(line); m != nil {
			units = append(units, Unit{Location: "slow limit", Name: m[1], Kind: KindTest})
			continue
		}
		switch {
		case line == "":
		case strings.HasPrefix(line, "== "):
			inFail, location = false, ""
		case strings.HasPrefix(line, "FAIL ") && strings.Contains(line, ":"):
			inFail, location = true, ""
		case !inFail:
		case line[0] != ' ':
			location = line
		case strings.HasPrefix(line, "  ") && line[2] != ' ' && location != "":
			units = append(units, Unit{Location: location, Name: strings.TrimSpace(line), Kind: KindTest})
		case len(units) > 0:
			u := &units[len(units)-1]
			u.Text = strings.TrimPrefix(u.Text+"\n"+strings.TrimSpace(line), "\n")
		}
	}
	return units
}
