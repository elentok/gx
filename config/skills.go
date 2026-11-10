package config

const defaultImplementSkill = "gx-implement"

var defaultCodeReviewSkills = []string{"gx-thermo-nuclear-code-quality-review", "gx-test-review"}

// SkillsConfig controls which skills gx-implement-equivalent work and code
// review run under.
type SkillsConfig struct {
	Implement  string   `json:"implement"`
	CodeReview []string `json:"code-review"`
	// CIFix lists the skills a ci-fix ticket runs. Empty means the bundled default.
	CIFix []string `json:"ci-fix"`
	// PRText is the skill that writes PR text. Empty means the bundled default.
	PRText string `json:"pr-text"`
}

// DefaultSkillsConfig returns the skills defaults.
func DefaultSkillsConfig() SkillsConfig {
	return SkillsConfig{
		Implement:  defaultImplementSkill,
		CodeReview: append([]string(nil), defaultCodeReviewSkills...),
		CIFix:      []string{},
	}
}
