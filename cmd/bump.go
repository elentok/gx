package cmd

import (
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/elentok/gx/ui"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func runBump(args []string, d deps) error {
	cwd, err := d.getwd()
	if err != nil {
		return err
	}

	lastTag, err := gitOutput(cwd, "describe", "--tags", "--abbrev=0")
	if err != nil {
		lastTag = "v0.0.0"
	}

	major, minor, patch, err := parseVersion(lastTag)
	if err != nil {
		return err
	}

	var bump string
	if len(args) == 1 && (args[0] == "major" || args[0] == "minor" || args[0] == "patch") {
		bump = args[0]
	} else {
		bump, err = pickBump(lastTag, major, minor, patch, d.stdin, d.stdout)
		if err != nil {
			return err
		}
		if bump == "" {
			return nil // cancelled
		}
	}

	switch bump {
	case "major":
		major++
		minor = 0
		patch = 0
	case "minor":
		minor++
		patch = 0
	case "patch":
		patch++
	}

	newTag := fmt.Sprintf("v%d.%d.%d", major, minor, patch)
	fmt.Fprintf(d.stdout, "Bumping %s → %s\n", lastTag, newTag)

	if err := gitRun(cwd, "tag", "-a", newTag, "-m", "Release "+newTag); err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}
	fmt.Fprintf(d.stdout, "Created annotated tag %s\n\n", newTag)

	confirmed, err := d.confirmForce("Push commits and tag to origin?")
	if err != nil {
		return err
	}
	if confirmed {
		if err := gitRun(cwd, "push", "origin"); err != nil {
			return err
		}
		if err := gitRun(cwd, "push", "origin", newTag); err != nil {
			return err
		}
		fmt.Fprintln(d.stdout, "Pushed.")
	} else {
		fmt.Fprintln(d.stdout, "Skipped. To push manually:")
		fmt.Fprintln(d.stdout, "  git push origin")
		fmt.Fprintf(d.stdout, "  git push origin %s\n", newTag)
	}
	return nil
}

// parseVersion parses a "vMAJOR.MINOR.PATCH" tag into its components.
func parseVersion(tag string) (major, minor, patch int, err error) {
	parts := strings.SplitN(strings.TrimPrefix(tag, "v"), ".", 3)
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("cannot parse version from tag %q", tag)
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	patch, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, fmt.Errorf("cannot parse version from tag %q", tag)
	}
	return major, minor, patch, nil
}

// pickBump shows an interactive menu and returns the chosen bump type, or ""
// if the user cancelled.
func pickBump(lastTag string, major, minor, patch int, in io.Reader, out io.Writer) (string, error) {
	m := bumpPickerModel{
		lastTag: lastTag,
		options: []bumpOption{
			{"patch", fmt.Sprintf("v%d.%d.%d", major, minor, patch+1)},
			{"minor", fmt.Sprintf("v%d.%d.%d", major, minor+1, 0)},
			{"major", fmt.Sprintf("v%d.%d.%d", major+1, 0, 0)},
		},
	}
	p := tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(out))
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	return final.(bumpPickerModel).chosen, nil
}

type bumpOption struct {
	kind   string
	newTag string
}

type bumpPickerModel struct {
	lastTag string
	options []bumpOption
	cursor  int
	chosen  string
	done    bool
}

func (m bumpPickerModel) Init() tea.Cmd { return nil }

func (m bumpPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case "enter":
			m.chosen = m.options[m.cursor].kind
			m.done = true
			return m, tea.Quit
		case "q", "ctrl+c", "esc":
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

var (
	stylePickerSelected   = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57")).Bold(true)
	stylePickerUnselected = lipgloss.NewStyle().Foreground(ui.ColorGray)
	stylePickerNewTag     = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	stylePickerOldTag     = lipgloss.NewStyle().Foreground(ui.ColorGray)
)

func (m bumpPickerModel) View() tea.View {
	if m.done {
		return tea.NewView("")
	}
	var b strings.Builder
	b.WriteString(ui.StyleBold.Render("gx bump"))
	b.WriteString("\n\n")
	b.WriteString("  Current: ")
	b.WriteString(stylePickerOldTag.Render(m.lastTag))
	b.WriteString("\n\n")
	for i, opt := range m.options {
		arrow := "  "
		label := fmt.Sprintf("%-7s  %s → %s", opt.kind, m.lastTag, opt.newTag)
		if i == m.cursor {
			b.WriteString(stylePickerSelected.Render("> " + label))
		} else {
			b.WriteString(arrow)
			b.WriteString(stylePickerUnselected.Render(fmt.Sprintf("%-7s  %s → ", opt.kind, m.lastTag)))
			b.WriteString(stylePickerNewTag.Render(opt.newTag))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(ui.StyleDim.Render("  " + ui.RenderInlineBindings(
		key.NewBinding(key.WithHelp("↑/↓ or j/k", "choose")),
		key.NewBinding(key.WithHelp("enter", "confirm")),
		key.NewBinding(key.WithHelp("q/esc", "cancel")),
	)))
	b.WriteString("\n")
	return tea.NewView(b.String())
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func gitRun(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run()
}
