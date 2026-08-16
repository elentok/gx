package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultLogConfig(t *testing.T) {
	cfg := DefaultLogConfig()
	if len(cfg.ImportantRefs) == 0 {
		t.Error("expected non-empty ImportantRefs")
	}
	found := false
	for _, rule := range cfg.ImportantRefs {
		for _, p := range rule.Patterns {
			if p == "^main$" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected ^main$ pattern in DefaultLogConfig")
	}
}

func TestLoadMissingUsesDefaults(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.UseNerdFontIcons {
		t.Fatal("UseNerdFontIcons = false, want true")
	}
	if cfg.StageDiffContextLines != 1 {
		t.Fatalf("StageDiffContextLines = %d, want 1", cfg.StageDiffContextLines)
	}
	if cfg.ExecutionQueue.MaxConcurrentTicketsPerEpic != 2 || cfg.ExecutionQueue.MaxConcurrentEpics != 2 {
		t.Fatalf("ExecutionQueue = %+v, want both limits to default to 2", cfg.ExecutionQueue)
	}
}

func TestLoadExecutionQueueConfigPreservesUnspecifiedDefault(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"execution-queue":{"max-concurrent-tickets-per-epic":4}}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ExecutionQueue.MaxConcurrentTicketsPerEpic != 4 {
		t.Fatalf("MaxConcurrentTicketsPerEpic = %d, want 4", cfg.ExecutionQueue.MaxConcurrentTicketsPerEpic)
	}
	if cfg.ExecutionQueue.MaxConcurrentEpics != 2 {
		t.Fatalf("MaxConcurrentEpics = %d, want default 2", cfg.ExecutionQueue.MaxConcurrentEpics)
	}
}

func TestLoadSubscriptionConfigDefaultsToUnsuppressed(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Subscription.SuppressExtraUsageWarning {
		t.Fatal("SuppressExtraUsageWarning = true, want false by default")
	}
}

func TestLoadSubscriptionConfigSuppressWarning(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"subscription":{"suppress-extra-usage-warning":true}}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Subscription.SuppressExtraUsageWarning {
		t.Fatal("SuppressExtraUsageWarning = false, want true")
	}
}

func TestLoadExecutionQueueConfigClampsLimitsToOne(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"execution-queue":{"max-concurrent-tickets-per-epic":0,"max-concurrent-epics":-3}}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ExecutionQueue.MaxConcurrentTicketsPerEpic != 1 || cfg.ExecutionQueue.MaxConcurrentEpics != 1 {
		t.Fatalf("ExecutionQueue = %+v, want both limits clamped to 1", cfg.ExecutionQueue)
	}
}

func writeBudgetConfig(t *testing.T, tmp string, body string) {
	t.Helper()
	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestLoadBudgetConfigDefaultsWhenAbsent(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Budget.SoftLimit != 20 || cfg.Budget.HardLimit != 30 {
		t.Fatalf("Budget limits = %+v, want soft=20 hard=30", cfg.Budget)
	}
	if want := []float64{5, 10, 15}; !floatSlicesEqual(cfg.Budget.NotificationThresholds, want) {
		t.Fatalf("Budget.NotificationThresholds = %v, want %v", cfg.Budget.NotificationThresholds, want)
	}
}

func TestLoadBudgetConfigZeroDisablesLimitsIndependently(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	writeBudgetConfig(t, tmp, `{"budget":{"soft-limit":0,"hard-limit":30}}`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Budget.SoftLimit != 0 {
		t.Fatalf("Budget.SoftLimit = %v, want 0 (disabled)", cfg.Budget.SoftLimit)
	}
	if cfg.Budget.HardLimit != 30 {
		t.Fatalf("Budget.HardLimit = %v, want 30 (unaffected)", cfg.Budget.HardLimit)
	}

	tmp2 := t.TempDir()
	userConfigDirFn = func() (string, error) { return tmp2, nil }
	writeBudgetConfig(t, tmp2, `{"budget":{"soft-limit":20,"hard-limit":0}}`)

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg2.Budget.HardLimit != 0 {
		t.Fatalf("Budget.HardLimit = %v, want 0 (disabled)", cfg2.Budget.HardLimit)
	}
	if cfg2.Budget.SoftLimit != 20 {
		t.Fatalf("Budget.SoftLimit = %v, want 20 (unaffected)", cfg2.Budget.SoftLimit)
	}
}

func TestLoadBudgetConfigSortsAndDedupesThresholds(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	writeBudgetConfig(t, tmp, `{"budget":{"notification-thresholds":[10,5,10,15,5]}}`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := []float64{5, 10, 15}; !floatSlicesEqual(cfg.Budget.NotificationThresholds, want) {
		t.Fatalf("Budget.NotificationThresholds = %v, want %v", cfg.Budget.NotificationThresholds, want)
	}
}

func TestLoadBudgetConfigBumpsHardLimitAtOrBelowSoftLimit(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	writeBudgetConfig(t, tmp, `{"budget":{"soft-limit":20,"hard-limit":20}}`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Budget.HardLimit != 20 {
		t.Fatalf("Budget.HardLimit = %v, want 20 (bumped to soft limit)", cfg.Budget.HardLimit)
	}

	tmp2 := t.TempDir()
	userConfigDirFn = func() (string, error) { return tmp2, nil }
	writeBudgetConfig(t, tmp2, `{"budget":{"soft-limit":20,"hard-limit":10}}`)

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg2.Budget.HardLimit != 20 {
		t.Fatalf("Budget.HardLimit = %v, want 20 (bumped to soft limit)", cfg2.Budget.HardLimit)
	}
}

func floatSlicesEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestLoadSkillsConfigDefaultsWhenBlockAbsent(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Skills.Implement != "gx-implement" {
		t.Fatalf("Skills.Implement = %q, want gx-implement", cfg.Skills.Implement)
	}
	if len(cfg.Skills.CodeReview) != 1 || cfg.Skills.CodeReview[0] != "thermo-nuclear-code-quality-review" {
		t.Fatalf("Skills.CodeReview = %+v, want [thermo-nuclear-code-quality-review]", cfg.Skills.CodeReview)
	}
}

func TestLoadSkillsConfigPreservesUnspecifiedDefault(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"skills":{"implement":"my-implement-skill"}}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Skills.Implement != "my-implement-skill" {
		t.Fatalf("Skills.Implement = %q, want my-implement-skill", cfg.Skills.Implement)
	}
	if len(cfg.Skills.CodeReview) != 1 || cfg.Skills.CodeReview[0] != "thermo-nuclear-code-quality-review" {
		t.Fatalf("Skills.CodeReview = %+v, want default [thermo-nuclear-code-quality-review]", cfg.Skills.CodeReview)
	}
}

func TestLoadSkillsConfigFullySpecified(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	body := `{"skills":{"implement":"custom-implement","code-review":["review-a","review-b"]}}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Skills.Implement != "custom-implement" {
		t.Fatalf("Skills.Implement = %q, want custom-implement", cfg.Skills.Implement)
	}
	if len(cfg.Skills.CodeReview) != 2 || cfg.Skills.CodeReview[0] != "review-a" || cfg.Skills.CodeReview[1] != "review-b" {
		t.Fatalf("Skills.CodeReview = %+v, want [review-a review-b]", cfg.Skills.CodeReview)
	}
}

func TestLoadParsesUseNerdFontIcons(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"use-nerdfont-icons":true}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.UseNerdFontIcons {
		t.Fatal("UseNerdFontIcons = false, want true")
	}
}

func TestLoadParsesStageDiffContextLines(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"stage-diff-context-lines":3}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.StageDiffContextLines != 3 {
		t.Fatalf("StageDiffContextLines = %d, want 3", cfg.StageDiffContextLines)
	}
}

func TestLoadClampsStageDiffContextLines(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"stage-diff-context-lines":999}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.StageDiffContextLines != 20 {
		t.Fatalf("StageDiffContextLines = %d, want 20", cfg.StageDiffContextLines)
	}
}

func TestInitCreatesDefaultConfig(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	path, err := Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) == "" {
		t.Fatal("expected non-empty config file")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.UseNerdFontIcons {
		t.Fatal("UseNerdFontIcons = false, want true by default")
	}
	if cfg.StageDiffContextLines != 1 {
		t.Fatalf("StageDiffContextLines = %d, want 1 by default", cfg.StageDiffContextLines)
	}
}

func TestLoadInputModalBottomNumeric(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"input-modal-bottom":10}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.InputModalBottom.Kind != InputModalBottomKindLines || cfg.InputModalBottom.Lines != 10 {
		t.Fatalf("got %+v, want Lines=10", cfg.InputModalBottom)
	}
}

func TestLoadInputModalBottomPercent(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"input-modal-bottom":"20%"}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.InputModalBottom.Kind != InputModalBottomKindPercent || cfg.InputModalBottom.Percent != 20 {
		t.Fatalf("got %+v, want Percent=20", cfg.InputModalBottom)
	}
}

func TestLoadInputModalBottomCenter(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"input-modal-bottom":"center"}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.InputModalBottom.Kind != InputModalBottomKindCenter {
		t.Fatalf("got %+v, want Center", cfg.InputModalBottom)
	}
}

func TestLoadInputModalBottomMissingUsesDefault(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	d := DefaultInputModalBottom()
	if cfg.InputModalBottom.Kind != d.Kind || cfg.InputModalBottom.Percent != d.Percent {
		t.Fatalf("got %+v, want default %+v", cfg.InputModalBottom, d)
	}
}

func TestLoadInputModalBottomInvalidFallsBackToDefault(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"input-modal-bottom":"bogus"}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	d := DefaultInputModalBottom()
	if cfg.InputModalBottom.Kind != d.Kind || cfg.InputModalBottom.Percent != d.Percent {
		t.Fatalf("got %+v, want default %+v", cfg.InputModalBottom, d)
	}
}

func TestLoadNameAliases(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"name-aliases":{"my-project-frontend":"project-fe"}}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.NameAliases["my-project-frontend"]; got != "project-fe" {
		t.Fatalf("NameAliases lookup = %q, want %q", got, "project-fe")
	}
}

func TestLoadLogHideRefsPreservesDefaultImportantRefs(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"log":{"hide-refs":["refs/remotes/origin/HEAD"]}}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Log.HideRefs) != 1 || cfg.Log.HideRefs[0] != "refs/remotes/origin/HEAD" {
		t.Fatalf("HideRefs = %v, want [refs/remotes/origin/HEAD]", cfg.Log.HideRefs)
	}
	def := DefaultLogConfig()
	if len(cfg.Log.ImportantRefs) != len(def.ImportantRefs) {
		t.Fatalf("ImportantRefs len = %d, want %d (defaults preserved)", len(cfg.Log.ImportantRefs), len(def.ImportantRefs))
	}
}

func TestLoadTelegramNotifications(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"notifications":{"telegram":{"bot-token":"abc123","chat-id":"456"}}}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Notifications.Telegram.BotToken != "abc123" {
		t.Fatalf("BotToken = %q, want %q", cfg.Notifications.Telegram.BotToken, "abc123")
	}
	if cfg.Notifications.Telegram.ChatID != "456" {
		t.Fatalf("ChatID = %q, want %q", cfg.Notifications.Telegram.ChatID, "456")
	}
}

func TestLoadTelegramNotificationsMissingUsesDefaults(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Notifications.Telegram.BotToken != "" {
		t.Fatalf("BotToken = %q, want empty", cfg.Notifications.Telegram.BotToken)
	}
	if cfg.Notifications.Telegram.ChatID != "" {
		t.Fatalf("ChatID = %q, want empty", cfg.Notifications.Telegram.ChatID)
	}
}

func TestLoadSlackNotifications(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"notifications":{"slack":{"webhook-url":"https://hooks.example.com/x"}}}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Notifications.Slack.WebhookURL != "https://hooks.example.com/x" {
		t.Fatalf("WebhookURL = %q, want %q", cfg.Notifications.Slack.WebhookURL, "https://hooks.example.com/x")
	}
}

func TestLoadSlackNotificationsMissingUsesDefaults(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Notifications.Slack.WebhookURL != "" {
		t.Fatalf("WebhookURL = %q, want empty", cfg.Notifications.Slack.WebhookURL)
	}
}

func TestDefaultAgentsConfig(t *testing.T) {
	cfg := DefaultAgentsConfig()
	if cfg.Claude.Model != "claude-sonnet-5" || cfg.Claude.Effort != "medium" {
		t.Fatalf("Claude = %+v, want claude-sonnet-5/medium", cfg.Claude)
	}
	if cfg.Codex.Model != "gpt-5.6-sol" || cfg.Codex.Effort != "medium" {
		t.Fatalf("Codex = %+v, want gpt-5.6-sol/medium", cfg.Codex)
	}
}

func TestLoadAgentsConfigDefaultsWhenBlockAbsent(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Agents.Claude.Model != "claude-sonnet-5" || cfg.Agents.Claude.Effort != "medium" {
		t.Fatalf("Agents.Claude = %+v, want claude-sonnet-5/medium", cfg.Agents.Claude)
	}
	if cfg.Agents.Codex.Model != "gpt-5.6-sol" || cfg.Agents.Codex.Effort != "medium" {
		t.Fatalf("Agents.Codex = %+v, want gpt-5.6-sol/medium", cfg.Agents.Codex)
	}
}

func TestLoadAgentsConfigPartialFillsMissingKeysFromDefaults(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	body := `{"agents":{"claude":{"model":"opus"}}}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Agents.Claude.Model != "opus" {
		t.Fatalf("Agents.Claude.Model = %q, want opus", cfg.Agents.Claude.Model)
	}
	if cfg.Agents.Claude.Effort != "medium" {
		t.Fatalf("Agents.Claude.Effort = %q, want default medium", cfg.Agents.Claude.Effort)
	}
	if cfg.Agents.Codex.Model != "gpt-5.6-sol" || cfg.Agents.Codex.Effort != "medium" {
		t.Fatalf("Agents.Codex = %+v, want untouched defaults", cfg.Agents.Codex)
	}
}

func TestLoadAgentsConfigExplicitEmptyStringMeansInherit(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	body := `{"agents":{"claude":{"model":"","effort":""}}}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Agents.Claude.Model != "" {
		t.Fatalf("Agents.Claude.Model = %q, want empty (inherit)", cfg.Agents.Claude.Model)
	}
	if cfg.Agents.Claude.Effort != "" {
		t.Fatalf("Agents.Claude.Effort = %q, want empty (inherit)", cfg.Agents.Claude.Effort)
	}
}

func TestLoadAgentsConfigTrimsWhitespace(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	body := `{"agents":{"codex":{"model":"  gpt-5.6-sol  ","effort":" high "}}}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Agents.Codex.Model != "gpt-5.6-sol" {
		t.Fatalf("Agents.Codex.Model = %q, want trimmed gpt-5.6-sol", cfg.Agents.Codex.Model)
	}
	if cfg.Agents.Codex.Effort != "high" {
		t.Fatalf("Agents.Codex.Effort = %q, want trimmed high", cfg.Agents.Codex.Effort)
	}
}

func TestInitFailsIfConfigExists(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	if _, err := Init(); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	if _, err := Init(); err == nil {
		t.Fatal("expected error on second Init, got nil")
	}
}
