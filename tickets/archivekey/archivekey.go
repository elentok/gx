// Package archivekey builds and reads archive keys: the epic part of a ticket
// or epic address, re-keyed to live under ".archive". It imports nothing from
// tickets, so tickets itself stays under its size limits.
package archivekey

import "strings"

// epicPrefix starts the epic part of an archive key. Archive names never start
// with a dot, so no active epic can collide with it.
const epicPrefix = ".archive/"

// New re-keys addr (an epic or ticket address) to the archive-only key
// project:.archive/<archiveName>, keeping a /NN ticket suffix. Only the epic
// part changes, so a new active epic that reuses the name never matches it.
func New(addr, archiveName string) string {
	epicPart, suffix, hasSuffix := strings.Cut(addr, "/")
	project, _, _ := strings.Cut(epicPart, ":")
	key := project + ":" + epicPrefix + archiveName
	if hasSuffix {
		key += "/" + suffix
	}
	return key
}

// Is reports whether s is an archive key (see New).
func Is(s string) bool {
	_, ok := Address(s)
	return ok
}

// Address reads the address back from an archive key: the archive name becomes
// the epic name and a /NN suffix is kept. It reports false for anything that is
// not an archive key.
func Address(key string) (string, bool) {
	project, rest, ok := strings.Cut(key, ":")
	if !ok || project == "" {
		return "", false
	}
	rest, ok = strings.CutPrefix(rest, epicPrefix)
	if !ok || rest == "" || strings.HasPrefix(rest, "/") {
		return "", false
	}
	return project + ":" + rest, true
}

// Epic drops a /NN ticket suffix from an archive key, leaving the epic's key.
// tickets.EpicAddress cannot do this: it cuts at the first slash, which inside
// an archive key is the one after ".archive".
func Epic(key string) string {
	head, rest, ok := strings.Cut(key, epicPrefix)
	if !ok {
		return key
	}
	name, _, _ := strings.Cut(rest, "/")
	return head + epicPrefix + name
}
