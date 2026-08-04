package skills

import "testing"

func TestDecide(t *testing.T) {
	cases := []struct {
		name string
		h    PathHashes
		mode InstallMode
		want Ownership
	}{
		{"never owned, nothing on disk", PathHashes{}, ModeManagedCopy, OwnershipAbsent},
		{"never owned, something on disk", PathHashes{Current: "x"}, ModeManagedCopy, OwnershipUnrelatedCollision},
		{"owned, nothing on disk", PathHashes{Installed: "x"}, ModeManagedCopy, OwnershipAbsent},
		{"owned, matches", PathHashes{Installed: "x", Current: "x"}, ModeManagedCopy, OwnershipUnchanged},
		{"owned, diverged, copy mode", PathHashes{Installed: "x", Current: "y"}, ModeManagedCopy, OwnershipModified},
		{"owned, diverged, symlink mode", PathHashes{Installed: "x", Current: "y"}, ModeSymlink, OwnershipWrongSymlinkTarget},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Decide(c.h, c.mode)
			if got != c.want {
				t.Errorf("Decide(%+v, %v) = %v, want %v", c.h, c.mode, got, c.want)
			}
		})
	}
}

func TestEvaluateCoversUnionOfPaths(t *testing.T) {
	installed := map[string]string{
		"unchanged.md": "a",
		"modified.md":  "a",
		"gone.md":      "a",
	}
	current := map[string]string{
		"unchanged.md":     "a",
		"modified.md":      "b",
		"new-collision.md": "c",
	}

	got := Evaluate(installed, current, ModeManagedCopy)

	want := map[string]Ownership{
		"unchanged.md":     OwnershipUnchanged,
		"modified.md":      OwnershipModified,
		"gone.md":          OwnershipAbsent,
		"new-collision.md": OwnershipUnrelatedCollision,
	}
	if len(got) != len(want) {
		t.Fatalf("Evaluate returned %d paths, want %d: %+v", len(got), len(want), got)
	}
	for path, wantOwnership := range want {
		if got[path] != wantOwnership {
			t.Errorf("Evaluate[%q] = %v, want %v", path, got[path], wantOwnership)
		}
	}
}

func TestForcePolicyAllowsOnlyNamedPaths(t *testing.T) {
	force := NewForcePolicy("a.md", "b.md")

	if !force.Allows("a.md") {
		t.Error("Allows(a.md) = false, want true")
	}
	if force.Allows("c.md") {
		t.Error("Allows(c.md) = true, want false")
	}

	var zero ForcePolicy
	if zero.Allows("a.md") {
		t.Error("zero-value ForcePolicy.Allows(a.md) = true, want false")
	}
}

func TestAllowWrite(t *testing.T) {
	force := NewForcePolicy("modified.md")

	cases := []struct {
		name      string
		ownership Ownership
		path      string
		want      bool
	}{
		{"absent always allowed", OwnershipAbsent, "any.md", true},
		{"unchanged always allowed", OwnershipUnchanged, "any.md", true},
		{"modified refused without force", OwnershipModified, "other.md", false},
		{"modified allowed when forced", OwnershipModified, "modified.md", true},
		{"unrelated collision refused without force", OwnershipUnrelatedCollision, "other.md", false},
		{"wrong symlink target refused without force", OwnershipWrongSymlinkTarget, "other.md", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := AllowWrite(c.ownership, c.path, force)
			if got != c.want {
				t.Errorf("AllowWrite(%v, %q, force) = %v, want %v", c.ownership, c.path, got, c.want)
			}
		})
	}
}
