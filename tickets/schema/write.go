package schema

import (
	"os"
	"path/filepath"
)

// UpdateTicket reads path's frontmatter ticket, applies mutate to it,
// validates the result, and — only if valid — writes it back atomically,
// leaving the body untouched. Nothing is written if the mutated ticket fails
// Validate; its error is returned as-is. This is the schema-package
// counterpart to ralphloop's private updateTicket, exported so cmd (the
// `gx tickets set` CLI) doesn't need to import ralphloop just to reuse its
// write path.
func UpdateTicket(path string, mutate func(*Ticket)) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	t, err := ParseTicketFromRaw(string(raw), path)
	if err != nil {
		return err
	}
	body := ParseBody(string(raw))

	mutate(&t)

	if err := Validate(t); err != nil {
		return err
	}

	out, err := MarshalTicket(t, body)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, out)
}

// ClearIterationStatus removes path's iteration_status, leaving Status and
// everything else untouched. This is the primitive ralph-loop's reattach
// wiring calls once a fresh agent session picks a ticket back up.
func ClearIterationStatus(path string) error {
	return UpdateTicket(path, func(t *Ticket) {
		t.IterationStatus = ""
	})
}

// writeFileAtomic replaces path's content via a same-directory temp file
// plus rename, so a concurrent reader never observes a torn/truncated write
// from an in-place os.WriteFile. Duplicated from ralphloop's writeFileAtomic
// (a ~15-line helper) rather than exported cross-package, per
// .scratch/ralph-tickets-visibility/issues/02-tickets-set-cli.md's Answer.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0644); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}
