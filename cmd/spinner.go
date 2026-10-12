package cmd

import (
	"io"
	"os"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-isatty"
)

type spinnerDoneMsg struct{ err error }

type spinnerModel struct {
	sp     spinner.Model
	label  string
	err    error
	doneCh <-chan error
}

func newSpinnerModel(label string, doneCh <-chan error) spinnerModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return spinnerModel{
		sp:     sp,
		label:  label,
		doneCh: doneCh,
	}
}

func (m spinnerModel) Init() tea.Cmd {
	return tea.Batch(m.sp.Tick, waitForDone(m.doneCh))
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.sp, cmd = m.sp.Update(msg)
		return m, cmd
	case spinnerDoneMsg:
		m.err = msg.err
		return m, tea.Quit
	}
	return m, nil
}

func (m spinnerModel) View() tea.View {
	return tea.NewView("  " + m.sp.View() + " " + m.label + "\n")
}

func waitForDone(doneCh <-chan error) tea.Cmd {
	return func() tea.Msg {
		return spinnerDoneMsg{err: <-doneCh}
	}
}

func runWithSpinner(in io.Reader, out io.Writer, label string, fn func() error) error {
	if !isTerminalWriter(out) {
		return fn()
	}
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- fn()
	}()

	p := tea.NewProgram(newSpinnerModel(label, doneCh), tea.WithInput(in), tea.WithOutput(out))
	finalModel, err := p.Run()
	if err != nil {
		return err
	}
	return finalModel.(spinnerModel).err
}

func isTerminalWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && isTerminalFile(f)
}

func isTerminalFile(f *os.File) bool {
	fd := f.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
