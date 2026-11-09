package registry

import "testing"

func tinyRegistry() *Registry {
	return New(
		Entry{Key: Key("server", "port"), Type: TypeNumber, Section: SectionServer},
		Entry{Key: Key("evolve.deep-read-cap"), Type: TypeNumber, Section: SectionEvolveRecovery, ProjectOverridable: true},
		Entry{Key: Key("queue", "max-agents"), Type: TypeNumber, Section: SectionExecutionQueue,
			ProjectOverridable: true, ProjectKey: Key("max-agents")},
		Entry{Key: Key("checks"), Type: TypeChecks, Section: SectionChecks, ProjectOnly: true},
		Entry{Key: Key("token"), Type: TypeSecret, Section: SectionServer},
	)
}

func TestLookupByKeyPath(t *testing.T) {
	r := tinyRegistry()
	e, ok := r.Lookup(Key("queue", "max-agents"))
	if !ok || e.ProjectKey.String() != "max-agents" {
		t.Fatalf("lookup = %+v, %v", e, ok)
	}
	if _, ok := r.Lookup(Key("queue")); ok {
		t.Fatal("prefix must not match")
	}
}

func TestEntriesInSectionOrder(t *testing.T) {
	var got []Section
	for _, e := range tinyRegistry().All() {
		got = append(got, e.Section)
	}
	want := []Section{SectionExecutionQueue, SectionServer, SectionServer, SectionEvolveRecovery, SectionChecks}
	if len(got) != len(want) {
		t.Fatalf("sections = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sections = %v, want %v", got, want)
		}
	}
	if n := len(tinyRegistry().InSection(SectionServer)); n != 2 {
		t.Fatalf("server entries = %d", n)
	}
	if SectionExecutionQueue.String() != "Execution queue" || SectionTicketStore.IsGlobal() != true || SectionChecks.IsGlobal() {
		t.Fatal("section names or global flag wrong")
	}
}

func TestFlatDottedKeyStaysOneSegment(t *testing.T) {
	k := Key("evolve.deep-read-cap")
	if len(k) != 1 {
		t.Fatalf("segments = %d", len(k))
	}
	if k.String() != "evolve.deep-read-cap" {
		t.Fatalf("String = %q", k.String())
	}
	if _, ok := tinyRegistry().Lookup(k); !ok {
		t.Fatal("flat key not found")
	}
	if _, ok := tinyRegistry().Lookup(Key("evolve", "deep-read-cap")); ok {
		t.Fatal("split key must not match the flat key")
	}
}

func TestProjectViewAndWriteOnly(t *testing.T) {
	r := tinyRegistry()
	proj := r.ForProject()
	if len(proj) != 3 {
		t.Fatalf("project entries = %d", len(proj))
	}
	for _, e := range proj {
		if !e.ProjectOverridable && !e.ProjectOnly {
			t.Fatalf("%v leaked into project view", e.Key)
		}
	}
	tok, _ := r.Lookup(Key("token"))
	port, _ := r.Lookup(Key("server", "port"))
	if !tok.WriteOnly() || port.WriteOnly() {
		t.Fatal("write-only must be exactly secret entries")
	}
}
