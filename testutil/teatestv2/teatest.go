// See
package teatestv2

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type Program interface {
	Send(tea.Msg)
}

type TestModelOptions struct {
	width  int
	height int
}

type TestOption func(opts *TestModelOptions)

func WithInitialTermSize(x, y int) TestOption {
	return func(opts *TestModelOptions) {
		opts.width = x
		opts.height = y
	}
}

type WaitingForContext struct {
	Duration      time.Duration
	CheckInterval time.Duration
}

type WaitForOption func(*WaitingForContext)

func WithCheckInterval(d time.Duration) WaitForOption {
	return func(wf *WaitingForContext) {
		wf.CheckInterval = d
	}
}

func WithDuration(d time.Duration) WaitForOption {
	return func(wf *WaitingForContext) {
		wf.Duration = d
	}
}

func WaitFor(tb testing.TB, r io.Reader, condition func(bts []byte) bool, options ...WaitForOption) {
	tb.Helper()
	if err := doWaitFor(r, condition, options...); err != nil {
		tb.Fatal(err)
	}
}

func doWaitFor(r io.Reader, condition func(bts []byte) bool, options ...WaitForOption) error {
	wf := WaitingForContext{
		Duration:      time.Second,
		CheckInterval: 20 * time.Millisecond,
	}
	for _, opt := range options {
		opt(&wf)
	}

	var b bytes.Buffer
	start := time.Now()
	for time.Since(start) <= wf.Duration {
		if _, err := io.ReadAll(io.TeeReader(r, &b)); err != nil {
			return fmt.Errorf("WaitFor: %w", err)
		}
		if condition(b.Bytes()) {
			return nil
		}
		time.Sleep(wf.CheckInterval)
	}
	return fmt.Errorf("WaitFor: condition not met after %s. Last output:\n%s", wf.Duration, b.String())
}

type TestModel struct {
	program *tea.Program

	in  *bytes.Buffer
	out io.ReadWriter

	modelCh chan tea.Model
	model   tea.Model

	done   sync.Once
	doneCh chan bool
}

type trackingModel struct {
	inner tea.Model
	out   io.ReadWriter
}

func (m trackingModel) Init() tea.Cmd {
	return m.inner.Init()
}

func (m trackingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.inner.Update(msg)
	return trackingModel{inner: next, out: m.out}, cmd
}

func (m trackingModel) View() tea.View {
	v := m.inner.View()
	_, _ = m.out.Write([]byte(v.Content))
	_, _ = m.out.Write([]byte("\n"))
	return v
}

func NewTestModel(tb testing.TB, m tea.Model, options ...TestOption) *TestModel {
	tb.Helper()

	tm := &TestModel{
		in:      bytes.NewBuffer(nil),
		out:     safe(bytes.NewBuffer(nil)),
		modelCh: make(chan tea.Model, 1),
		doneCh:  make(chan bool, 1),
	}

	var opts TestModelOptions
	for _, opt := range options {
		opt(&opts)
	}

	programOpts := []tea.ProgramOption{
		tea.WithInput(tm.in),
		tea.WithOutput(io.Discard),
		tea.WithoutSignals(),
	}
	if opts.width > 0 && opts.height > 0 {
		programOpts = append(programOpts, tea.WithWindowSize(opts.width, opts.height))
	}

	tm.program = tea.NewProgram(trackingModel{inner: m, out: tm.out}, programOpts...)

	interruptions := make(chan os.Signal, 1)
	signal.Notify(interruptions, syscall.SIGINT)
	go func() {
		model, err := tm.program.Run()
		if err != nil {
			tb.Fatalf("app failed: %s", err)
		}
		if tracked, ok := model.(trackingModel); ok {
			model = tracked.inner
		}
		tm.modelCh <- model
		tm.doneCh <- true
	}()
	go func() {
		<-interruptions
		signal.Stop(interruptions)
		tb.Log("interrupted")
		tm.program.Kill()
	}()

	return tm
}

func (tm *TestModel) waitDone(tb testing.TB, opts []FinalOpt) {
	tm.done.Do(func() {
		fopts := FinalOpts{}
		for _, opt := range opts {
			opt(&fopts)
		}
		if fopts.timeout > 0 {
			select {
			case <-time.After(fopts.timeout):
				if fopts.onTimeout == nil {
					tb.Fatalf("timeout after %s", fopts.timeout)
				}
				fopts.onTimeout(tb)
			case <-tm.doneCh:
			}
		} else {
			<-tm.doneCh
		}
	})
}

type FinalOpts struct {
	timeout   time.Duration
	onTimeout func(tb testing.TB)
}

type FinalOpt func(opts *FinalOpts)

func WithTimeoutFn(fn func(tb testing.TB)) FinalOpt {
	return func(opts *FinalOpts) {
		opts.onTimeout = fn
	}
}

func WithFinalTimeout(d time.Duration) FinalOpt {
	return func(opts *FinalOpts) {
		opts.timeout = d
	}
}

func (tm *TestModel) WaitFinished(tb testing.TB, opts ...FinalOpt) {
	tm.waitDone(tb, opts)
}

func (tm *TestModel) FinalModel(tb testing.TB, opts ...FinalOpt) tea.Model {
	tm.waitDone(tb, opts)
	select {
	case m := <-tm.modelCh:
		if m != nil {
			tm.model = m
		}
		return tm.model
	default:
		return tm.model
	}
}

func (tm *TestModel) Output() io.Reader {
	return tm.out
}

func (tm *TestModel) Send(m tea.Msg) {
	tm.program.Send(m)
}

func (tm *TestModel) Quit() error {
	tm.program.Quit()
	return nil
}

func (tm *TestModel) Type(s string) {
	for _, r := range s {
		tm.Send(tea.KeyPressMsg{
			Code: r,
			Text: string(r),
		})
	}
}

func safe(rw io.ReadWriter) io.ReadWriter {
	return &safeReadWriter{rw: rw}
}

type safeReadWriter struct {
	rw io.ReadWriter
	m  sync.RWMutex
}

func (s *safeReadWriter) Read(p []byte) (n int, err error) {
	s.m.RLock()
	defer s.m.RUnlock()
	return s.rw.Read(p)
}

func (s *safeReadWriter) Write(p []byte) (int, error) {
	s.m.Lock()
	defer s.m.Unlock()
	return s.rw.Write(p)
}
