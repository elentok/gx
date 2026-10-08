package tickets

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// allocLockFileName is the per-epic ticket-ID allocation lock (see
// LockEpic): at most one caller may be computing-and-writing a new ticket's
// ID at a time for a given epic, while unrelated epics never contend with
// each other.
const allocLockFileName = ".alloc.lock"

// LockEpic acquires an exclusive, filesystem-scoped lock for epicPath —
// callers should hold it for the whole "read existing tickets, compute the
// next ID, write the new ticket file" sequence (see NextTicketID) so two
// concurrent callers targeting the same epic never compute the same ID.
// epicPath must already exist as a directory; LockEpic does not create it.
// The returned unlock func must be called (typically via defer) to release
// the lock.
func LockEpic(epicPath string) (unlock func(), err error) {
	lockPath := filepath.Join(epicPath, allocLockFileName)
	deadline := time.Now().Add(10 * time.Second)

	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			f.Close()
			return func() { os.Remove(lockPath) }, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("acquiring allocation lock at %s: %w", lockPath, err)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for allocation lock at %s", lockPath)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// LoadLockedEpic resolves epicPath (cleaned), acquires its LockEpic, loads
// the epic set rooted at epicPath's parent directory, and returns the one
// matching epicPath's base name — all under the lock, so the returned epic's
// ticket list is safe to compute a new ticket ID against. On any error the
// lock (if acquired) is released before returning, so the returned unlock is
// always nil and safe to ignore in that case; callers should still defer
// unlock() unconditionally when err is nil.
func LoadLockedEpic(epicPath string) (epic *Epic, unlock func(), err error) {
	epicPath = filepath.Clean(epicPath)
	scratchDir := filepath.Dir(epicPath)
	epicName := filepath.Base(epicPath)

	if info, statErr := os.Stat(epicPath); statErr != nil || !info.IsDir() {
		return nil, nil, fmt.Errorf("epic not found: %s", epicPath)
	}

	unlock, err = LockEpic(epicPath)
	if err != nil {
		return nil, nil, err
	}

	epics, err := Load(scratchDir)
	if err != nil {
		unlock()
		return nil, nil, fmt.Errorf("loading epics under %s: %w", scratchDir, err)
	}
	for i := range epics {
		if epics[i].Name == epicName {
			return &epics[i], unlock, nil
		}
	}

	unlock()
	return nil, nil, fmt.Errorf("epic not found: %s", epicPath)
}

// parentIDRe splits a parent ID argument into its digits, optional single
// trailing letter, and — only past that letter — optional trailing digits,
// mirroring schema.TicketID's shape.
var parentIDRe = regexp.MustCompile(`^(\d{2,})([a-z]?)(\d*)$`)

// NextTicketID computes the next ticket ID to allocate within epic, given an
// optional parent. It is a pure computation over epic.Tickets (as already
// loaded, e.g. by Load) — callers needing atomicity across concurrent
// processes must reload the epic and call this while holding LockEpic.
//
// parent == "": the next flat sibling number, e.g. "04" after "01".."03".
// parent is a bare number (e.g. "12"): the next lettered child, "12a" then
// "12b". parent has a letter (e.g. "12b" or, past its own trailing digits,
// "12b1"): the next number under that letter, "12b1" then "12b2" — both
// spellings of the same lettered ancestor converge on the same next ID.
func NextTicketID(epic Epic, parent string) (string, error) {
	if parent == "" {
		max := 0
		for _, t := range epic.Tickets {
			if t.Number > max {
				max = t.Number
			}
		}
		return fmt.Sprintf("%02d", max+1), nil
	}

	m := parentIDRe.FindStringSubmatch(parent)
	if m == nil {
		return "", fmt.Errorf("invalid parent ticket ID %q", parent)
	}
	digits, letter := m[1], m[2]

	if letter == "" {
		return digits + string(nextLetter(epic.Tickets, digits)), nil
	}

	prefix := digits + letter
	return fmt.Sprintf("%s%d", prefix, nextNumber(epic.Tickets, prefix)), nil
}

// IDParent is the fork parent id's spelling implies: "12" for "12a", "12b"
// for "12b1". ok is false for an ID that is not a fork child.
func IDParent(id string) (parent string, ok bool) {
	m := parentIDRe.FindStringSubmatch(strings.ToLower(id))
	if m == nil || m[2] == "" {
		return "", false
	}
	if m[3] == "" {
		return m[1], true
	}
	return m[1] + m[2], true
}

// anyPaddingIDRe is parentIDRe for a parent token, whose padding the
// parent resolver ignores (see siblingKey).
var anyPaddingIDRe = regexp.MustCompile(`^(\d+)([a-z]?)(\d*)$`)

// ParentAllowed reports whether parent may be id's fork parent: IDParent(id),
// or an earlier number under the same letter, since NextTicketID numbers a
// fork of "12b1" under "12b".
func ParentAllowed(id, parent string) bool {
	if _, ok := IDParent(id); !ok {
		return false
	}
	c := parentIDRe.FindStringSubmatch(strings.ToLower(id))
	p := anyPaddingIDRe.FindStringSubmatch(strings.ToLower(parent))
	if p == nil {
		return false
	}
	cNum, _ := strconv.Atoi(c[1])
	pNum, _ := strconv.Atoi(p[1])
	switch {
	case cNum != pNum:
		return false
	case p[2] == "":
		return c[3] == ""
	case p[2] != c[2] || c[3] == "":
		return false
	case p[3] == "":
		return true
	}
	cSeq, _ := strconv.Atoi(c[3])
	pSeq, _ := strconv.Atoi(p[3])
	return pSeq < cSeq
}

// nextLetter returns the next unused single-letter suffix after prefix
// among existing (e.g. "b" when "12a" already exists under prefix "12"),
// starting at "a" when none exist yet.
func nextLetter(existing []Ticket, prefix string) byte {
	max := byte(0)
	for _, t := range existing {
		suffix, ok := strings.CutPrefix(t.Identifier, prefix)
		if !ok || len(suffix) != 1 {
			continue
		}
		if c := suffix[0]; c >= 'a' && c <= 'z' && c > max {
			max = c
		}
	}
	if max == 0 {
		return 'a'
	}
	return max + 1
}

// nextNumber returns the next unused numeric suffix after prefix among
// existing (e.g. 2 when "12b1" already exists under prefix "12b"), starting
// at 1 when none exist yet.
func nextNumber(existing []Ticket, prefix string) int {
	max := 0
	for _, t := range existing {
		suffix, ok := strings.CutPrefix(t.Identifier, prefix)
		if !ok || suffix == "" {
			continue
		}
		n, err := strconv.Atoi(suffix)
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
	}
	return max + 1
}
