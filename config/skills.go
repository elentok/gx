package config

const defaultImplementSkill = "gx-implement"

var defaultCodeReviewSkills = []string{"thermo-nuclear-code-quality-review"}

// SkillsConfig controls which skills gx-implement-equivalent work and code
// review run under.
type SkillsConfig struct {
	Implement  string   `json:"implement"`
	CodeReview []string `json:"code-review"`
}

// DefaultSkillsConfig returns the skills defaults.
func DefaultSkillsConfig() SkillsConfig {
	return SkillsConfig{
		Implement:  defaultImplementSkill,
		CodeReview: append([]string(nil), defaultCodeReviewSkills...),
	}
}
