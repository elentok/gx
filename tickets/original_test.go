package tickets

import (
	"errors"
	"path/filepath"
	"testing"
)

func writeArchived(t *testing.T, dir, id, resolution string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, ArchivedOneOffsEpic, "issues", id+"-one-off.md"),
		"---\nid: \""+id+"\"\nstatus: done\ntype: implement\n---\n\n## Resolution\n\n"+resolution)
}

func TestFindOriginal(t *testing.T) {
	dir := t.TempDir()
	writeArchived(t, dir, "01", "- original: p:one-offs/03\n")
	writeArchived(t, dir, "02", "- original: p:one-offs/04\n")
	writeArchived(t, dir, "03", "- original: p:one-offs/05\n")
	writeArchived(t, dir, "04", "- original: p:one-offs/05\n")

	t.Run("found", func(t *testing.T) {
		got, tk, err := FindOriginal("p", dir, Address{"p", "one-offs", "03"})
		if err != nil || got.String() != "p:one-offs-archived/01" || tk.Identifier != "01" {
			t.Fatalf("got %v, %q, %v", got, tk.Identifier, err)
		}
	})
	t.Run("not found", func(t *testing.T) {
		_, _, err := FindOriginal("p", dir, Address{"p", "one-offs", "09"})
		var ae *AddressError
		if !errors.As(err, &ae) || ae.Code != CodeUnknownTicket {
			t.Fatalf("err = %v, want %s", err, CodeUnknownTicket)
		}
	})
	t.Run("ambiguous", func(t *testing.T) {
		_, _, err := FindOriginal("p", dir, Address{"p", "one-offs", "05"})
		var ae *AddressError
		if !errors.As(err, &ae) || ae.Code != CodeAmbiguousOriginal {
			t.Fatalf("err = %v, want %s", err, CodeAmbiguousOriginal)
		}
	})
}
