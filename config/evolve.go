package config

// Evolve modes: what the retro hook does when an epic completes.
const (
	EvolveOff         = "off"
	EvolveMaintainer  = "maintainer"
	EvolveContributor = "contributor"
)

// DefaultEvolveDeepReadCap is the EvolveDeepReadCap when none is configured.
const DefaultEvolveDeepReadCap = 8

func validEvolveMode(mode string) bool {
	switch mode {
	case EvolveOff, EvolveMaintainer, EvolveContributor:
		return true
	}
	return false
}

// EvolveMode is the effective evolve mode for the project pf: off when the
// project opted out with `evolve: false`, otherwise the global mode.
func (c Config) EvolveMode(pf ProjectFile) string {
	if pf.Evolve != nil && !*pf.Evolve {
		return EvolveOff
	}
	return c.Evolve
}

// EvolveEnabled reports whether evolve runs for the project pf.
func (c Config) EvolveEnabled(pf ProjectFile) bool {
	return c.EvolveMode(pf) != EvolveOff
}
